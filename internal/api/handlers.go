package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/JustJay7/court-data-fetcher/internal/cache"
	"github.com/JustJay7/court-data-fetcher/internal/config"
	"github.com/JustJay7/court-data-fetcher/internal/database"
	"github.com/JustJay7/court-data-fetcher/internal/scraper"
	"github.com/JustJay7/court-data-fetcher/pkg/logger"
	"gorm.io/gorm"
)

// Handlers holds all HTTP handlers
type Handlers struct {
	db      *gorm.DB
	cache   cache.Cache
	scraper *scraper.Scraper
	logger  *logger.Logger
	cfg     *config.Config
}

// NewHandlers creates a new handlers instance
func NewHandlers(db *gorm.DB, cache cache.Cache, scraper *scraper.Scraper, logger *logger.Logger, cfg *config.Config) *Handlers {
	return &Handlers{
		db:      db,
		cache:   cache,
		scraper: scraper,
		logger:  logger,
		cfg:     cfg,
	}
}

// HomePage renders the home page
func (h *Handlers) HomePage(c *gin.Context) {
	// Debug: Log when home page is accessed
	h.logger.Info("Home page accessed", "ip", c.ClientIP())
	
	c.HTML(http.StatusOK, "index.html", gin.H{
		"title":     "Court Data Fetcher",
		"courtName": h.cfg.CourtName,
		"caseTypes": getCaseTypes(),
		"years":     getYearRange(),
	})
}

// SearchCase handles case search form submission
func (h *Handlers) SearchCase(c *gin.Context) {
	var req struct {
		CaseType   string `form:"case_type" binding:"required"`
		CaseNumber string `form:"case_number" binding:"required"`
		FilingYear string `form:"filing_year" binding:"required"`
	}

	if err := c.ShouldBind(&req); err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{
			"error": "Invalid form data: " + err.Error(),
		})
		return
	}

	// Check cache first
	cacheKey := cache.GenerateCacheKey(req.CaseType, req.CaseNumber, req.FilingYear)
	if cachedCase, found := h.cache.Get(cacheKey); found {
		h.logger.Info("Cache hit", "key", cacheKey)
		
		// Load the query log for this cached case
		var queryLog database.QueryLog
		h.db.Where("id = ?", cachedCase.QueryLogID).First(&queryLog)
		
		c.HTML(http.StatusOK, "results.html", gin.H{
			"case":      cachedCase,
			"queryLog":  queryLog,
			"fromCache": true,
		})
		return
	}

	// Create query log entry BEFORE scraping
	queryLog := &database.QueryLog{
		CaseType:   req.CaseType,
		CaseNumber: req.CaseNumber,
		FilingYear: req.FilingYear,
		QueryTime:  time.Now(),
		IPAddress:  c.ClientIP(),
	}

	// Save initial query log
	if err := h.db.Create(queryLog).Error; err != nil {
		h.logger.Error("Failed to create query log", "error", err)
	}

	// Perform scraping
	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.ScraperTimeout)
	defer cancel()

	caseInfo, rawHTML, err := h.scraper.SearchCase(ctx, req.CaseType, req.CaseNumber, req.FilingYear)
	
	// Update query log with results
	queryLog.RawResponse = rawHTML
	if err != nil {
		queryLog.Success = false
		queryLog.ErrorMessage = err.Error()
		h.db.Save(queryLog)
		
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "Failed to fetch case data: " + err.Error(),
		})
		return
	}

	// Update query log as successful
	queryLog.Success = true
	h.db.Save(queryLog)

	// Save case info to database with all relationships
	caseInfo.QueryLogID = queryLog.ID
	if err := h.db.Create(caseInfo).Error; err != nil {
		h.logger.Error("Failed to save case info", "error", err)
	}

	// Save parties
	for i := range caseInfo.Parties {
		caseInfo.Parties[i].CaseInfoID = caseInfo.ID
		h.db.Create(&caseInfo.Parties[i])
	}

	// Save orders
	for i := range caseInfo.Orders {
		caseInfo.Orders[i].CaseInfoID = caseInfo.ID
		h.db.Create(&caseInfo.Orders[i])
	}

	// Cache the result
	h.cache.Set(cacheKey, caseInfo)

	// Render results with query log
	c.HTML(http.StatusOK, "results.html", gin.H{
		"case":      caseInfo,
		"queryLog":  queryLog,
		"fromCache": false,
	})
}

// ViewResults displays saved results
func (h *Handlers) ViewResults(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{
			"error": "Invalid result ID",
		})
		return
	}

	// Try to find by CaseInfo ID first
	var caseInfo database.CaseInfo
	if err := h.db.Preload("Parties").Preload("Orders").First(&caseInfo, id).Error; err != nil {
		// If not found, try to find by QueryLog ID
		var queryLog database.QueryLog
		if err := h.db.First(&queryLog, id).Error; err != nil {
			c.HTML(http.StatusNotFound, "error.html", gin.H{
				"error": "Result not found",
			})
			return
		}
		
		// Load CaseInfo from QueryLog
		if err := h.db.Where("query_log_id = ?", queryLog.ID).
			Preload("Parties").
			Preload("Orders").
			First(&caseInfo).Error; err != nil {
			c.HTML(http.StatusNotFound, "error.html", gin.H{
				"error": "Case information not found for this query",
			})
			return
		}
		
		c.HTML(http.StatusOK, "results.html", gin.H{
			"case":     &caseInfo,
			"queryLog": &queryLog,
		})
		return
	}

	// Load the associated query log
	var queryLog database.QueryLog
	if caseInfo.QueryLogID > 0 {
		h.db.First(&queryLog, caseInfo.QueryLogID)
	}

	c.HTML(http.StatusOK, "results.html", gin.H{
		"case":     &caseInfo,
		"queryLog": &queryLog,
	})
}

// GetCaseAPI handles API requests for case information
func (h *Handlers) GetCaseAPI(c *gin.Context) {
	caseType := c.Query("type")
	caseNumber := c.Query("number")
	filingYear := c.Query("year")

	if caseType == "" || caseNumber == "" || filingYear == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Missing required parameters: type, number, year",
		})
		return
	}

	// Check cache
	cacheKey := cache.GenerateCacheKey(caseType, caseNumber, filingYear)
	if cachedCase, found := h.cache.Get(cacheKey); found {
		c.JSON(http.StatusOK, gin.H{
			"success":   true,
			"data":      cachedCase,
			"fromCache": true,
		})
		return
	}

	// Perform scraping
	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.ScraperTimeout)
	defer cancel()

	caseInfo, _, err := h.scraper.SearchCase(ctx, caseType, caseNumber, filingYear)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Cache result
	h.cache.Set(cacheKey, caseInfo)

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      caseInfo,
		"fromCache": false,
	})
}

// ListCasesAPI returns all cached cases
func (h *Handlers) ListCasesAPI(c *gin.Context) {
	var cases []database.CaseInfo
	
	// Get pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset := (page - 1) * limit

	// Get total count
	var total int64
	h.db.Model(&database.CaseInfo{}).Count(&total)

	// Fetch cases
	h.db.Preload("Parties").Preload("Orders").
		Offset(offset).Limit(limit).
		Order("created_at DESC").
		Find(&cases)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    cases,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// BulkSearchAPI handles bulk case searches
func (h *Handlers) BulkSearchAPI(c *gin.Context) {
	var req struct {
		Queries []scraper.CaseQuery `json:"queries" binding:"required,min=1,max=10"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.ScraperTimeout*time.Duration(len(req.Queries)))
	defer cancel()

	results, err := h.scraper.SearchCaseConcurrent(ctx, req.Queries)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Format results
	var responseData []gin.H
	for _, result := range results {
		data := gin.H{
			"query": result.Query,
		}

		if result.Error != nil {
			data["success"] = false
			data["error"] = result.Error.Error()
		} else {
			data["success"] = true
			data["data"] = result.CaseInfo
		}

		responseData = append(responseData, data)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"results": responseData,
	})
}

// HealthCheck returns the health status
func (h *Handlers) HealthCheck(c *gin.Context) {
	// Check database connection
	var count int64
	dbHealthy := h.db.Model(&database.QueryLog{}).Count(&count).Error == nil

	c.JSON(http.StatusOK, gin.H{
		"status":   "healthy",
		"database": dbHealthy,
		"cache":    h.cache.Stats(),
		"time":     time.Now().Unix(),
	})
}

// CacheStats returns cache statistics
func (h *Handlers) CacheStats(c *gin.Context) {
	stats := h.cache.Stats()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stats":   stats,
	})
}

// CaptchaPage renders the CAPTCHA solving page
func (h *Handlers) CaptchaPage(c *gin.Context) {
	c.HTML(http.StatusOK, "captcha.html", gin.H{
		"title": "Solve CAPTCHA",
	})
}

// GetCaptcha returns CAPTCHA image for manual solving
func (h *Handlers) GetCaptcha(c *gin.Context) {
	captchaID := c.Param("id")
	
	// Read CAPTCHA image
	captchaPath := fmt.Sprintf("./data/captchas/%s.png", captchaID)
	data, err := os.ReadFile(captchaPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "CAPTCHA not found",
		})
		return
	}
	
	// Return image
	c.Data(http.StatusOK, "image/png", data)
}

// SolveCaptcha accepts manual CAPTCHA solution
func (h *Handlers) SolveCaptcha(c *gin.Context) {
	captchaID := c.Param("id")
	
	var req struct {
		Solution string `json:"solution" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request",
		})
		return
	}
	
	// Save solution
	solutionPath := fmt.Sprintf("./data/captchas/%s.txt", captchaID)
	if err := os.WriteFile(solutionPath, []byte(req.Solution), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save solution",
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "CAPTCHA solution saved",
	})
}

// DownloadPDF proxies PDF download from court website
func (h *Handlers) DownloadPDF(c *gin.Context) {
	pdfURL := c.Query("url")
	if pdfURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "PDF URL required",
		})
		return
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create request with proper headers
	req, err := http.NewRequest("GET", pdfURL, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid URL",
		})
		return
	}

	// Add headers to appear like a browser
	req.Header.Set("User-Agent", h.cfg.UserAgent)
	req.Header.Set("Referer", h.cfg.CourtBaseURL)

	// Fetch the PDF
	resp, err := client.Do(req)
	if err != nil {
		h.logger.Error("Failed to fetch PDF", "url", pdfURL, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to download PDF",
		})
		return
	}
	defer resp.Body.Close()

	// Check if response is OK
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "PDF not found",
		})
		return
	}

	// Extract filename from URL or use default
	filename := "court_order.pdf"
	urlParts := strings.Split(pdfURL, "/")
	if len(urlParts) > 0 {
		lastPart := urlParts[len(urlParts)-1]
		if strings.HasSuffix(lastPart, ".pdf") {
			filename = lastPart
		}
	}

	// Set headers for file download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/pdf")

	// Stream the PDF to client
	written, err := io.Copy(c.Writer, resp.Body)
	if err != nil {
		h.logger.Error("Failed to stream PDF", "error", err)
		return
	}

	h.logger.Info("PDF downloaded", "url", pdfURL, "size", written)
}

// GetQueryLogs returns paginated query logs
func (h *Handlers) GetQueryLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset := (page - 1) * limit

	var logs []database.QueryLog
	var total int64

	// Get total count
	h.db.Model(&database.QueryLog{}).Count(&total)

	// Fetch logs
	h.db.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&logs)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    logs,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

// GetRawResponse returns the raw HTML response from a query log
func (h *Handlers) GetRawResponse(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid ID",
		})
		return
	}

	var queryLog database.QueryLog
	if err := h.db.First(&queryLog, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Query log not found",
		})
		return
	}

	// Return raw HTML
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, queryLog.RawResponse)
}

// ViewLogs displays query logs page
func (h *Handlers) ViewLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit := 20
	offset := (page - 1) * limit

	var logs []database.QueryLog
	var total int64

	// Get total count
	h.db.Model(&database.QueryLog{}).Count(&total)

	// Fetch logs
	h.db.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&logs)

	totalPages := int(total) / limit
	if int(total)%limit > 0 {
		totalPages++
	}

	// Calculate prev and next pages
	prevPage := page - 1
	nextPage := page + 1

	c.HTML(http.StatusOK, "logs.html", gin.H{
		"title": "Query Logs",
		"logs":  logs,
		"pagination": gin.H{
			"page":       page,
			"limit":      limit,
			"total":      total,
			"totalPages": totalPages,
			"prevPage":   prevPage,
			"nextPage":   nextPage,
		},
	})
}

// Helper functions

func getCaseTypes() []string {
	return []string{
		"BAIL APPLN.", "CS", "CC", "CRL.M.C", "CRL.A", "CRL.REV.P",
		"FAO", "RFA", "RSA", "CR", "EXEC",
	}
}

func getYearRange() []string {
	currentYear := time.Now().Year()
	years := make([]string, 0, 20)
	
	for year := currentYear; year >= currentYear-19; year-- {
		years = append(years, fmt.Sprintf("%d", year))
	}
	
	return years
}