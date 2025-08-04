package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/JustJay7/court-data-fetcher/internal/api"
	"github.com/JustJay7/court-data-fetcher/internal/cache"
	"github.com/JustJay7/court-data-fetcher/internal/config"
	"github.com/JustJay7/court-data-fetcher/internal/scraper"
	"github.com/JustJay7/court-data-fetcher/pkg/logger"
	"gorm.io/gorm"
)

type Server struct {
	cfg      *config.Config
	db       *gorm.DB
	cache    cache.Cache
	logger   *logger.Logger
	router   *gin.Engine
	scraper  *scraper.Scraper
}

func New(cfg *config.Config, db *gorm.DB, cache cache.Cache, logger *logger.Logger) *Server {
	if cfg.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	router.Use(gin.Recovery())
	router.Use(loggingMiddleware(logger))
	router.Use(corsMiddleware())

	scraperInstance, err := scraper.NewScraper(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to initialize scraper", "error", err)
	}

	server := &Server{
		cfg:     cfg,
		db:      db,
		cache:   cache,
		logger:  logger,
		router:  router,
		scraper: scraperInstance,
	}

	api.SetupRoutes(router, db, cache, scraperInstance, logger, cfg)

	return server
}

func (s *Server) Run() error {
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%s", s.cfg.Host, s.cfg.Port),
		Handler:      s.router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Fatal("Failed to start server", "error", err)
		}
	}()

	s.logger.Info("Server started", "address", srv.Addr)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	s.logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.scraper.Close(); err != nil {
		s.logger.Error("Failed to close scraper", "error", err)
	}

	if err := srv.Shutdown(ctx); err != nil {
		s.logger.Error("Server forced to shutdown", "error", err)
		return err
	}

	s.logger.Info("Server exited gracefully")
	return nil
}

func loggingMiddleware(logger *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		logger.Info("HTTP Request",
			"client_ip", clientIP,
			"method", method,
			"path", path,
			"status", statusCode,
			"latency", latency.String(),
			"user_agent", c.Request.UserAgent(),
		)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}