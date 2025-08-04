package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/JustJay7/court-data-fetcher/internal/cache"
	"github.com/JustJay7/court-data-fetcher/internal/config"
	"github.com/JustJay7/court-data-fetcher/internal/database"
	"github.com/JustJay7/court-data-fetcher/internal/scraper"
	"github.com/JustJay7/court-data-fetcher/internal/server"
	"github.com/JustJay7/court-data-fetcher/pkg/logger"
	"gorm.io/gorm"
)

func main() {
	// Parse command line flags
	var migrate bool
	flag.BoolVar(&migrate, "migrate", false, "Run database migrations")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log, err := logger.NewLogger(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	// Initialize database
	db, err := database.Initialize(cfg.DatabasePath)
	if err != nil {
		log.Fatal("Failed to initialize database", "error", err)
	}

	// Run migrations if requested
	if migrate {
		if err := database.Migrate(db); err != nil {
			log.Fatal("Failed to run migrations", "error", err)
		}
		log.Info("Database migrations completed successfully")
		return
	}

	// Initialize cache
	cacheService := cache.NewCache(cfg.CacheSize, cfg.CacheTTL)

	// Create and start server
	srv := server.New(cfg, db, cacheService, log)
	
	// Start PDF download worker in background
	go startPDFDownloadWorker(db, log, cfg)
	
	log.Info("Starting Court Data Fetcher", 
		"host", cfg.Host,
		"port", cfg.Port,
		"court", cfg.CourtName,
	)

	if err := srv.Run(); err != nil {
		log.Fatal("Server failed to start", "error", err)
	}
}

// startPDFDownloadWorker runs a background worker to download PDFs
func startPDFDownloadWorker(db *gorm.DB, log *logger.Logger, cfg *config.Config) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	downloader := scraper.NewPDFDownloader(db, log, cfg.DatabasePath)

	for {
		select {
		case <-ticker.C:
			log.Info("Starting PDF download job")
			if err := downloader.DownloadOrderPDFs(); err != nil {
				log.Error("PDF download job failed", "error", err)
			}
		}
	}
}