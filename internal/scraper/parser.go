package scraper

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/JustJay7/court-data-fetcher/internal/database"
	"github.com/JustJay7/court-data-fetcher/pkg/logger"
)

// Parser handles HTML parsing operations for Delhi District Courts
type Parser struct {
	logger *logger.Logger
}

// NewParser creates a new parser instance
func NewParser(logger *logger.Logger) *Parser {
	return &Parser{logger: logger}
}

// ParseCaseDetails parses case information from Delhi District Court results
func (p *Parser) ParseCaseDetails(page *rod.Page) (*database.CaseInfo, error) {
	caseInfo := &database.CaseInfo{}

	// Delhi District Courts typically shows case details in a specific format
	// Look for the case details container
	detailsContainer, err := page.Element("div.container, div#case_details, div.case-info")
	if err != nil {
		return nil, fmt.Errorf("case details container not found")
	}

	// Method 1: Try to parse from table format (common in e-Courts)
	if table, err := detailsContainer.Element("table"); err == nil {
		p.parseCaseDetailsFromTable(table, caseInfo)
	}

	// Method 2: Try to parse from div/span structure
	if caseInfo.CaseNumber == "" {
		p.parseCaseDetailsFromDivs(detailsContainer, caseInfo)
	}

	// Method 3: Parse from text patterns if structured parsing fails
	if caseInfo.CaseNumber == "" {
		bodyText, _ := detailsContainer.Text()
		p.parseCaseDetailsFromText(bodyText, caseInfo)
	}

	// Validate we got at least the case number
	if caseInfo.CaseNumber == "" {
		return nil, fmt.Errorf("failed to extract case number")
	}

	// Parse parties information
	parties, err := p.parseParties(page)
	if err != nil {
		p.logger.Warn("Failed to parse parties", "error", err)
	} else {
		caseInfo.Parties = parties
	}

	// Parse case status history if available
	p.parseCaseHistory(page, caseInfo)

	return caseInfo, nil
}

// parseCaseDetailsFromTable extracts case info from table format
func (p *Parser) parseCaseDetailsFromTable(table *rod.Element, caseInfo *database.CaseInfo) {
	rows := table.MustElements("tr")
	
	for _, row := range rows {
		cells := row.MustElements("td, th")
		if len(cells) >= 2 {
			label := strings.ToLower(strings.TrimSpace(cells[0].MustText()))
			value := strings.TrimSpace(cells[1].MustText())
			
			// Map common e-Courts labels to our fields
			switch {
			case strings.Contains(label, "case number") || strings.Contains(label, "cnr"):
				caseInfo.CaseNumber = value
			case strings.Contains(label, "case type"):
				caseInfo.CaseType = value
			case strings.Contains(label, "filing number"):
				// Extract year from filing number if not set
				if matches := regexp.MustCompile(`\d{4}`).FindString(value); matches != "" {
					caseInfo.FilingYear = matches
				}
			case strings.Contains(label, "filing date") || strings.Contains(label, "date of filing"):
				caseInfo.FilingDate, _ = p.parseDate(value)
			case strings.Contains(label, "registration date"):
				if caseInfo.FilingDate.IsZero() {
					caseInfo.FilingDate, _ = p.parseDate(value)
				}
			case strings.Contains(label, "next date") || strings.Contains(label, "next hearing"):
				caseInfo.NextHearing, _ = p.parseDate(value)
			case strings.Contains(label, "stage") || strings.Contains(label, "status"):
				caseInfo.Status = value
			case strings.Contains(label, "judge") || strings.Contains(label, "coram"):
				caseInfo.Judge = value
			case strings.Contains(label, "court") && !strings.Contains(label, "court number"):
				caseInfo.CourtComplex = value
			}
		}
	}
}

// parseCaseDetailsFromDivs extracts case info from div/span structure
func (p *Parser) parseCaseDetailsFromDivs(container *rod.Element, caseInfo *database.CaseInfo) {
	// Look for labeled spans or divs
	elements := container.MustElements("div, span")
	
	for i, elem := range elements {
		text := strings.TrimSpace(elem.MustText())
		lowerText := strings.ToLower(text)
		
		// Check if this element is a label
		if strings.HasSuffix(text, ":") {
			// Get the next element as value
			if i+1 < len(elements) {
				value := strings.TrimSpace(elements[i+1].MustText())
				
				switch {
				case strings.Contains(lowerText, "case no"):
					caseInfo.CaseNumber = value
				case strings.Contains(lowerText, "case type"):
					caseInfo.CaseType = value
				case strings.Contains(lowerText, "year"):
					caseInfo.FilingYear = value
				case strings.Contains(lowerText, "filing date"):
					caseInfo.FilingDate, _ = p.parseDate(value)
				case strings.Contains(lowerText, "next date"):
					caseInfo.NextHearing, _ = p.parseDate(value)
				case strings.Contains(lowerText, "status"):
					caseInfo.Status = value
				case strings.Contains(lowerText, "judge"):
					caseInfo.Judge = value
				}
			}
		}
	}
}

// parseCaseDetailsFromText uses regex patterns to extract info from text
func (p *Parser) parseCaseDetailsFromText(text string, caseInfo *database.CaseInfo) {
	// Case Number patterns
	caseNumberPatterns := []string{
		`Case\s*No[\.\s:]+([A-Z]+[\/\-]?\d+[\/\-]?\d+)`,
		`CNR\s*Number[\.\s:]+([A-Z0-9]+)`,
		`([A-Z]+[\/\-]\d+[\/\-]\d{4})`,
	}
	
	for _, pattern := range caseNumberPatterns {
		if matches := regexp.MustCompile(pattern).FindStringSubmatch(text); len(matches) > 1 {
			caseInfo.CaseNumber = matches[1]
			break
		}
	}
	
	// Extract case type from case number if not separately available
	if caseInfo.CaseType == "" && caseInfo.CaseNumber != "" {
		parts := regexp.MustCompile(`[\/\-]`).Split(caseInfo.CaseNumber, -1)
		if len(parts) > 0 {
			caseInfo.CaseType = parts[0]
		}
	}
	
	// Filing Year
	if matches := regexp.MustCompile(`Year[\.\s:]+(\d{4})`).FindStringSubmatch(text); len(matches) > 1 {
		caseInfo.FilingYear = matches[1]
	}
	
	// Dates
	datePattern := `(\d{1,2}[\-\/]\d{1,2}[\-\/]\d{4})`
	dates := regexp.MustCompile(datePattern).FindAllString(text, -1)
	
	// Try to identify dates by context
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, "filing date") || strings.Contains(lowerLine, "institution") {
			if date := regexp.MustCompile(datePattern).FindString(line); date != "" {
				caseInfo.FilingDate, _ = p.parseDate(date)
			}
		}
		if strings.Contains(lowerLine, "next") && strings.Contains(lowerLine, "date") {
			if date := regexp.MustCompile(datePattern).FindString(line); date != "" {
				caseInfo.NextHearing, _ = p.parseDate(date)
			}
		}
	}
}

// parseParties extracts party information from Delhi District Court format
func (p *Parser) parseParties(page *rod.Page) ([]database.Party, error) {
	var parties []database.Party
	
	// Method 1: Look for parties in table format
	partyTable, err := page.Element("table#party_table, table.party-table, div#party_details table")
	if err == nil {
		return p.parsePartiesFromTable(partyTable)
	}
	
	// Method 2: Look for parties in div structure
	partyContainer, err := page.Element("div#petitioner_respondent, div.party-details, div#party_info")
	if err == nil {
		return p.parsePartiesFromDivs(partyContainer)
	}
	
	// Method 3: Parse from text patterns
	bodyText, _ := page.Element("body").Text()
	return p.parsePartiesFromText(bodyText)
}

// parsePartiesFromTable extracts parties from table format
func (p *Parser) parsePartiesFromTable(table *rod.Element) ([]database.Party, error) {
	var parties []database.Party
	rows := table.MustElements("tr")
	
	// Skip header row
	for i := 1; i < len(rows); i++ {
		cells := rows[i].MustElements("td")
		if len(cells) >= 2 {
			party := database.Party{}
			
			// First cell usually contains party type
			partyType := strings.TrimSpace(cells[0].MustText())
			if strings.Contains(strings.ToLower(partyType), "petitioner") {
				party.Type = "Petitioner"
			} else if strings.Contains(strings.ToLower(partyType), "respondent") {
				party.Type = "Respondent"
			} else {
				party.Type = partyType
			}
			
			// Second cell contains party name
			party.Name = strings.TrimSpace(cells[1].MustText())
			
			// Additional cells may contain advocate info
			if len(cells) > 2 {
				party.AdvocateName = strings.TrimSpace(cells[2].MustText())
			}
			if len(cells) > 3 {
				party.AdvocateCode = strings.TrimSpace(cells[3].MustText())
			}
			
			if party.Name != "" {
				parties = append(parties, party)
			}
		}
	}
	
	return parties, nil
}

// parsePartiesFromDivs extracts parties from div structure
func (p *Parser) parsePartiesFromDivs(container *rod.Element) ([]database.Party, error) {
	var parties []database.Party
	
	// Look for petitioner section
	petitionerSection, err := container.Element("div.petitioner, div#petitioner")
	if err == nil {
		petitionerText := petitionerSection.MustText()
		petitioners := p.extractPartyNames(petitionerText)
		for _, name := range petitioners {
			parties = append(parties, database.Party{
				Type: "Petitioner",
				Name: name,
			})
		}
	}
	
	// Look for respondent section
	respondentSection, err := container.Element("div.respondent, div#respondent")
	if err == nil {
		respondentText := respondentSection.MustText()
		respondents := p.extractPartyNames(respondentText)
		for _, name := range respondents {
			parties = append(parties, database.Party{
				Type: "Respondent",
				Name: name,
			})
		}
	}
	
	return parties, nil
}

// parsePartiesFromText extracts parties using text patterns
func (p *Parser) parsePartiesFromText(text string) ([]database.Party, error) {
	var parties []database.Party
	
	// Look for patterns like "Petitioner: Name" or "Petitioner(s): Name"
	petitionerPattern := regexp.MustCompile(`(?i)Petitioner\(?\s?\)?:?\s*([^\n\r]+)`)
	if matches := petitionerPattern.FindStringSubmatch(text); len(matches) > 1 {
		names := p.extractPartyNames(matches[1])
		for _, name := range names {
			parties = append(parties, database.Party{
				Type: "Petitioner",
				Name: name,
			})
		}
	}
	
	respondentPattern := regexp.MustCompile(`(?i)Respondent\(?\s?\)?:?\s*([^\n\r]+)`)
	if matches := respondentPattern.FindStringSubmatch(text); len(matches) > 1 {
		names := p.extractPartyNames(matches[1])
		for _, name := range names {
			parties = append(parties, database.Party{
				Type: "Respondent",
				Name: name,
			})
		}
	}
	
	return parties, nil
}

// extractPartyNames splits multiple party names
func (p *Parser) extractPartyNames(text string) []string {
	var names []string
	
	// Clean up the text
	text = strings.TrimSpace(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	
	// Split by common separators
	parts := regexp.MustCompile(`\s+(?:and|AND|And|&)\s+`).Split(text, -1)
	
	for _, part := range parts {
		name := strings.TrimSpace(part)
		// Remove trailing "etc" or numbers
		name = regexp.MustCompile(`\s*(?:etc\.?|\d+\.?)$`).ReplaceAllString(name, "")
		if name != "" && len(name) > 2 {
			names = append(names, name)
		}
	}
	
	return names
}

// ParseOrders extracts order/judgment information from Delhi District Courts
func (p *Parser) ParseOrders(page *rod.Page) ([]database.Order, error) {
	var orders []database.Order
	
	// Try multiple selectors for orders table
	orderSelectors := []string{
		"table#order_table",
		"table.order-table",
		"div#order_details table",
		"table[summary*='order']",
		"table[summary*='Order']",
	}
	
	var ordersTable *rod.Element
	for _, selector := range orderSelectors {
		table, err := page.Element(selector)
		if err == nil {
			ordersTable = table
			break
		}
	}
	
	if ordersTable == nil {
		// Try to find any table that might contain orders
		tables := page.MustElements("table")
		for _, table := range tables {
			text := table.MustText()
			if strings.Contains(strings.ToLower(text), "order") && 
			   (strings.Contains(text, "PDF") || strings.Contains(text, "Download")) {
				ordersTable = table
				break
			}
		}
	}
	
	if ordersTable == nil {
		return orders, fmt.Errorf("orders table not found")
	}
	
	rows := ordersTable.MustElements("tr")
	for i := 1; i < len(rows); i++ { // Skip header
		cells := rows[i].MustElements("td")
		if len(cells) >= 2 {
			order := database.Order{}
			
			// Parse order date (usually first column)
			dateStr := strings.TrimSpace(cells[0].MustText())
			if dateStr != "" {
				order.OrderDate, _ = p.parseDate(dateStr)
			}
			
			// Parse description (usually second column)
			order.Description = strings.TrimSpace(cells[1].MustText())
			
			// Look for PDF link (usually in last column or as link in description)
			for _, cell := range cells {
				links := cell.MustElements("a")
				for _, link := range links {
					href, err := link.Attribute("href")
					if err == nil && href != nil {
						hrefStr := *href
						if strings.Contains(strings.ToLower(hrefStr), "pdf") ||
						   strings.Contains(strings.ToLower(hrefStr), "download") ||
						   strings.Contains(strings.ToLower(hrefStr), "order") {
							order.PDFLink = p.makeAbsoluteURL(page, hrefStr)
							break
						}
					}
				}
			}
			
			// Try to extract judge name if available
			if len(cells) > 2 {
				order.JudgeName = strings.TrimSpace(cells[2].MustText())
			}
			
			if order.OrderDate.Year() > 1900 { // Valid date check
				orders = append(orders, order)
			}
		}
	}
	
	return orders, nil
}

// parseCaseHistory extracts case history/status
func (p *Parser) parseCaseHistory(page *rod.Page, caseInfo *database.CaseInfo) {
	// Look for case history table
	historyTable, err := page.Element("table#case_history, table.case-history, div#history table")
	if err != nil {
		return
	}
	
	rows := historyTable.MustElements("tr")
	for _, row := range rows {
		text := strings.ToLower(row.MustText())
		// Look for disposal status
		if strings.Contains(text, "disposed") || strings.Contains(text, "decided") {
			caseInfo.Status = "Disposed"
			return
		}
		// Look for current status
		if strings.Contains(text, "pending") {
			caseInfo.Status = "Pending"
		}
	}
}

// parseDate parses various date formats used by Indian courts
func (p *Parser) parseDate(dateStr string) (time.Time, error) {
	// Clean the date string
	dateStr = strings.TrimSpace(dateStr)
	dateStr = regexp.MustCompile(`\s+`).ReplaceAllString(dateStr, " ")
	
	// Common date formats in Indian court systems
	formats := []string{
		"02-01-2006",
		"02/01/2006",
		"02.01.2006",
		"02-Jan-2006",
		"02-January-2006",
		"02 Jan 2006",
		"02 January 2006",
		"2006-01-02",
		"Jan 02, 2006",
		"January 02, 2006",
	}
	
	for _, format := range formats {
		if date, err := time.Parse(format, dateStr); err == nil {
			return date, nil
		}
	}
	
	// Try to handle some edge cases
	// Remove day names if present
	dateStr = regexp.MustCompile(`(?i)(Monday|Tuesday|Wednesday|Thursday|Friday|Saturday|Sunday),?\s*`).ReplaceAllString(dateStr, "")
	
	for _, format := range formats {
		if date, err := time.Parse(format, dateStr); err == nil {
			return date, nil
		}
	}
	
	return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}

// makeAbsoluteURL converts a relative URL to absolute
func (p *Parser) makeAbsoluteURL(page *rod.Page, relativeURL string) string {
	if strings.HasPrefix(relativeURL, "http://") || strings.HasPrefix(relativeURL, "https://") {
		return relativeURL
	}
	
	pageURL := page.MustInfo().URL
	
	// Parse base URL
	parts := strings.Split(pageURL, "/")
	if len(parts) >= 3 {
		baseURL := strings.Join(parts[:3], "/")
		
		if strings.HasPrefix(relativeURL, "/") {
			return baseURL + relativeURL
		} else {
			// Get directory path
			dirParts := parts[:len(parts)-1]
			return strings.Join(dirParts, "/") + "/" + relativeURL
		}
	}
	
	return relativeURL
}

// ParseError extracts error message from the page
func (p *Parser) ParseError(page *rod.Page) string {
	errorSelectors := []string{
		".error-message",
		".alert-danger",
		"#errorMsg",
		"div.error",
		"span.error",
		"div[style*='color:red']",
		"span[style*='color:red']",
	}

	for _, selector := range errorSelectors {
		elem, err := page.Element(selector)
		if err == nil && elem != nil {
			text := strings.TrimSpace(elem.MustText())
			if text != "" {
				return text
			}
		}
	}

	// Check for common error patterns in page text
	bodyText := page.MustElement("body").MustText()
	errorPhrases := []string{
		"No records found",
		"No Record Found",
		"Invalid case number",
		"Case not found",
		"No data available",
		"Wrong Captcha",
		"Invalid Captcha",
	}

	bodyLower := strings.ToLower(bodyText)
	for _, phrase := range errorPhrases {
		if strings.Contains(bodyLower, strings.ToLower(phrase)) {
			return phrase
		}
	}

	return ""
}