package api

import (
	"context"
	"fmt"
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
		c.HTML(http.StatusOK, "results.html", gin.H{
			"case":      cachedCase,
			"fromCache": true,
		})
		return
	}

	// Create query log
	queryLog := &database.QueryLog{
		CaseType:   req.CaseType,
		CaseNumber: req.CaseNumber,
		FilingYear: req.FilingYear,
		QueryTime:  time.Now(),
		IPAddress:  c.ClientIP(),
	}

	// Perform scraping
	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.ScraperTimeout)
	defer cancel()

	caseInfo, rawHTML, err := h.scraper.SearchCase(ctx, req.CaseType, req.CaseNumber, req.FilingYear)
	
	// Update query log
	queryLog.RawResponse = rawHTML
	if err != nil {
		queryLog.Success = false
		queryLog.ErrorMessage = err.Error()
		h.db.Create(queryLog)
		
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "Failed to fetch case data: " + err.Error(),
		})
		return
	}

	queryLog.Success = true
	h.db.Create(queryLog)

	// Save case info to database
	caseInfo.QueryLogID = queryLog.ID
	if err := h.db.Create(caseInfo).Error; err != nil {
		h.logger.Error("Failed to save case info", "error", err)
	}

	// Cache the result
	h.cache.Set(cacheKey, caseInfo)

	// Render results
	c.HTML(http.StatusOK, "results.html", gin.H{
		"case":      caseInfo,
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

	var caseInfo database.CaseInfo
	if err := h.db.Preload("Parties").Preload("Orders").First(&caseInfo, id).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{
			"error": "Result not found",
		})
		return
	}

	c.HTML(http.StatusOK, "results.html", gin.H{
		"case": &caseInfo,
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

// Helper functions

func getCaseTypes() []string {
	return []string{
		"CS", "CC", "CRL.M.C", "CRL.A", "CRL.REV.P",
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