package scraper

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/JustJay7/court-data-fetcher/internal/config"
	"github.com/JustJay7/court-data-fetcher/internal/database"
	"github.com/JustJay7/court-data-fetcher/pkg/logger"
)

// Scraper handles web scraping operations
type Scraper struct {
	cfg      *config.Config
	browser  *rod.Browser
	mu       sync.Mutex
	logger   *logger.Logger
	sessions map[string]*rod.Page
}

// NewScraper creates a new scraper instance
func NewScraper(cfg *config.Config, logger *logger.Logger) (*Scraper, error) {
	// Configure launcher with proper options
	l := launcher.New().
		Headless(cfg.HeadlessMode).
		Set("user-agent", cfg.UserAgent).
		Set("disable-blink-features", "AutomationControlled").
		Delete("enable-automation")

	// Set browser path if specified
	if cfg.BrowserPath != "" {
		l = l.Bin(cfg.BrowserPath)
	}

	// For debugging
	if cfg.LogLevel == "debug" {
		l = l.Devtools(true)
	}

	// Launch browser
	browserURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	browser := rod.New().ControlURL(browserURL).MustConnect()

	return &Scraper{
		cfg:      cfg,
		browser:  browser,
		logger:   logger,
		sessions: make(map[string]*rod.Page),
	}, nil
}

// Close closes the browser and all pages
func (s *Scraper) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, page := range s.sessions {
		page.MustClose()
	}

	return s.browser.Close()
}

// SearchCase searches for a case and returns the parsed information
func (s *Scraper) SearchCase(ctx context.Context, caseType, caseNumber, filingYear string) (*database.CaseInfo, string, error) {
	s.mu.Lock()
	page, err := s.getOrCreatePage(ctx)
	s.mu.Unlock()

	if err != nil {
		return nil, "", fmt.Errorf("failed to create page: %w", err)
	}

	// Create a timeout context
	searchCtx, cancel := context.WithTimeout(ctx, s.cfg.ScraperTimeout)
	defer cancel()

	// Navigate to Delhi District Court e-Courts website
	courtURL := s.cfg.CourtBaseURL + "/case-status"
	s.logger.Info("Navigating to court website", "url", courtURL)
	
	err = page.Context(searchCtx).Navigate(courtURL)
	if err != nil {
		return nil, "", fmt.Errorf("failed to navigate: %w", err)
	}

	// Wait for page to load completely
	err = page.WaitLoad()
	if err != nil {
		return nil, "", fmt.Errorf("page load timeout: %w", err)
	}

	// Wait for the case status tab/link
	s.logger.Debug("Looking for case status link")
	
	// Click on Case Status tab if not already there
	caseStatusLinks := page.MustElements("a")
	var caseStatusTab *rod.Element
	for _, link := range caseStatusLinks {
		text, _ := link.Text()
		if strings.Contains(text, "Case Status") {
			caseStatusTab = link
			break
		}
	}
	if caseStatusTab != nil {
		caseStatusTab.MustClick()
		page.MustWaitNavigation()
	}

	// Select search by Case Number
	searchBySelect := page.MustElement("select[name='search_by']")
	searchBySelect.MustSelect("casenumber")
	
	// Wait for form to update
	time.Sleep(1 * time.Second)

	// Fill the form with actual Delhi District Court fields
	s.logger.Debug("Filling search form")
	
	// Select District (for Delhi courts)
	districtSelect := page.MustElement("select[name='district_code']")
	districtSelect.MustSelect("1") // Delhi District code
	
	// Select Court Complex
	page.MustWaitIdle()
	complexSelect := page.MustElement("select[name='court_complex_code']")
	complexSelect.MustSelect("1") // Default complex
	
	// Enter case details
	caseTypeInput := page.MustElement("input[name='case_type']")
	caseTypeInput.MustInput(caseType)
	
	caseNumberInput := page.MustElement("input[name='case_no']")
	caseNumberInput.MustInput(caseNumber)
	
	caseYearInput := page.MustElement("input[name='case_year']")
	caseYearInput.MustInput(filingYear)

	// Handle CAPTCHA before submission
	if err := s.handleCaptcha(page); err != nil {
		return nil, "", fmt.Errorf("captcha handling failed: %w", err)
	}

	// Submit form
	s.logger.Debug("Submitting search form")
	submitBtn := page.MustElement("input[type='submit'][value='Search']")
	submitBtn.MustClick()

	// Wait for results
	page.MustWaitNavigation()
	time.Sleep(2 * time.Second)

	// Check for errors
	errorMsg := s.checkForErrors(page)
	if errorMsg != "" {
		return nil, "", fmt.Errorf("search error: %s", errorMsg)
	}

	// Get page HTML for logging
	html, _ := page.HTML()

	// Parse results
	caseInfo, err := s.parseResults(page)
	if err != nil {
		return nil, html, fmt.Errorf("failed to parse results: %w", err)
	}

	// Try to fetch additional details
	if err := s.fetchAdditionalDetails(page, caseInfo); err != nil {
		s.logger.Warn("Failed to fetch additional details", "error", err)
	}

	return caseInfo, html, nil
}

// getOrCreatePage gets an existing page or creates a new one
func (s *Scraper) getOrCreatePage(ctx context.Context) (*rod.Page, error) {
	sessionID := fmt.Sprintf("session_%d", time.Now().Unix())
	
	if page, exists := s.sessions[sessionID]; exists {
		return page, nil
	}

	page, err := s.browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, err
	}

	// Set viewport
	page.MustSetViewport(1920, 1080, 1, false)

	// Set extra headers to appear more human-like
	page.MustSetExtraHeaders("Accept-Language", "en-US,en;q=0.9")

	s.sessions[sessionID] = page
	return page, nil
}

// checkForErrors checks for error messages on the page
func (s *Scraper) checkForErrors(page *rod.Page) string {
	// Check for common error messages
	errorSelectors := []string{
		"div.error",
		"div.alert-danger",
		"span.error-message",
		"div#errormsg",
	}

	for _, selector := range errorSelectors {
		elem, err := page.Element(selector)
		if err == nil && elem != nil {
			text, _ := elem.Text()
			if text != "" {
				return text
			}
		}
	}

	// Check for "No records found" type messages
	bodyText, _ := page.Element("body")
	if bodyText != nil {
		text, _ := bodyText.Text()
		lowerText := strings.ToLower(text)
		if strings.Contains(lowerText, "no record") || 
		   strings.Contains(lowerText, "not found") ||
		   strings.Contains(lowerText, "invalid case") {
			return "No records found for the given case details"
		}
	}

	return ""
}

// parseResults parses the case information from the results page
func (s *Scraper) parseResults(page *rod.Page) (*database.CaseInfo, error) {
	parser := NewParser(s.logger)
	
	// First check if we're on the case details page
	// Delhi District Courts shows results in a table format
	resultsTable, err := page.Element("table.table")
	if err != nil {
		return nil, fmt.Errorf("no results table found")
	}

	// Click on View button to get full details
	viewBtn := resultsTable.MustElement("a[href*='view']")
	if viewBtn != nil {
		viewBtn.MustClick()
		page.MustWaitNavigation()
	}

	return parser.ParseCaseDetails(page)
}

// fetchAdditionalDetails fetches order details and other information
func (s *Scraper) fetchAdditionalDetails(page *rod.Page, caseInfo *database.CaseInfo) error {
	// Look for Orders/Judgments tab
	links := page.MustElements("a")
	var ordersTab *rod.Element
	
	for _, link := range links {
		text, _ := link.Text()
		if strings.Contains(text, "Orders") || strings.Contains(text, "Judgements") {
			ordersTab = link
			break
		}
	}

	if ordersTab != nil {
		ordersTab.MustClick()
		page.MustWaitNavigation()

		// Parse orders
		parser := NewParser(s.logger)
		orders, err := parser.ParseOrders(page)
		if err != nil {
			return err
		}
		caseInfo.Orders = orders
	}

	return nil
}

// SearchCaseConcurrent performs concurrent case searches
func (s *Scraper) SearchCaseConcurrent(ctx context.Context, queries []CaseQuery) ([]CaseResult, error) {
	results := make([]CaseResult, len(queries))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, s.cfg.MaxConcurrentScrapes)

	for i, query := range queries {
		wg.Add(1)
		go func(index int, q CaseQuery) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Perform search
			caseInfo, rawHTML, err := s.SearchCase(ctx, q.CaseType, q.CaseNumber, q.FilingYear)
			
			results[index] = CaseResult{
				Query:    q,
				CaseInfo: caseInfo,
				RawHTML:  rawHTML,
				Error:    err,
			}
		}(i, query)
	}

	wg.Wait()
	return results, nil
}

// CaseQuery represents a case search query
type CaseQuery struct {
	CaseType   string
	CaseNumber string
	FilingYear string
}

// CaseResult represents the result of a case search
type CaseResult struct {
	Query    CaseQuery
	CaseInfo *database.CaseInfo
	RawHTML  string
	Error    error
}