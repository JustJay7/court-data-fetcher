package api

import (
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

	// HTML routes
	router.GET("/", h.HomePage)
	router.POST("/search", h.SearchCase)
	router.GET("/results/:id", h.ViewResults)
	router.GET("/captcha", h.CaptchaPage)id", h.ViewResults)
	router.GET("/captcha", h.CaptchaPage)id", h.ViewResults)

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
	}

	// Load HTML templates
	router.LoadHTMLGlob("web/templates/*")
}