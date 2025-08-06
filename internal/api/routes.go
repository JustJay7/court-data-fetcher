package api

import (
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/JustJay7/court-data-fetcher/internal/cache"
	"github.com/JustJay7/court-data-fetcher/internal/config"
	"github.com/JustJay7/court-data-fetcher/internal/scraper"
	"github.com/JustJay7/court-data-fetcher/pkg/logger"
	"gorm.io/gorm"
)

// SetupRoutes configures all application routes
func SetupRoutes(router *gin.Engine, db *gorm.DB, cache cache.Cache, scraper *scraper.Scraper, logger *logger.Logger, cfg *config.Config) {
	// Create handlers
	h := NewHandlers(db, cache, scraper, logger, cfg)

	// Serve static files
	router.Static("/static", "./web/static")

	// Test endpoints
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Server is running",
			"time": time.Now().Unix(),
		})
	})
	
	// Simple test without scraper
	router.GET("/test-simple", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
			"court_url": cfg.CourtBaseURL,
			"court_name": cfg.CourtName,
		})
	})

	// HTML routes
	router.GET("/", h.HomePage)
	router.POST("/search", h.SearchCase)
	router.GET("/results/:id", h.ViewResults)
	router.GET("/captcha", h.CaptchaPage)
	router.GET("/logs", h.ViewLogs)

	// API routes
	api := router.Group("/api")
	{
		// Health check
		api.GET("/health", h.HealthCheck)

		// Case endpoints
		api.GET("/case", h.GetCaseAPI)
		api.GET("/cases", h.ListCasesAPI)
		
		// Cache stats
		api.GET("/cache/stats", h.CacheStats)
		
		// Concurrent search
		api.POST("/cases/bulk", h.BulkSearchAPI)
		
		// CAPTCHA endpoints
		api.GET("/captcha/:id", h.GetCaptcha)
		api.POST("/captcha/:id/solve", h.SolveCaptcha)
		
		// PDF download proxy
		api.GET("/download/pdf", h.DownloadPDF)
		
		// Query logs
		api.GET("/logs", h.GetQueryLogs)
		api.GET("/logs/:id/raw", h.GetRawResponse)
	}

	// Load HTML templates
	router.LoadHTMLGlob("web/templates/*")
	
	// Debug: List loaded templates
	router.GET("/debug/templates", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Templates should be loaded",
			"path": "web/templates/*",
		})
	})
}