package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/JustJay7/court-data-fetcher/internal/cache"
	"github.com/JustJay7/court-data-fetcher/internal/config"
	"github.com/JustJay7/court-data-fetcher/internal/database"
	"github.com/JustJay7/court-data-fetcher/internal/server"
	"github.com/JustJay7/court-data-fetcher/pkg/logger"
)

func main() {
	var migrate bool
	flag.BoolVar(&migrate, "migrate", false, "Run database migrations")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.NewLogger(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	db, err := database.Initialize(cfg.DatabasePath)
	if err != nil {
		log.Fatal("Failed to initialize database", "error", err)
	}

	if migrate {
		if err := database.Migrate(db); err != nil {
			log.Fatal("Failed to run migrations", "error", err)
		}
		log.Info("Database migrations completed successfully")
		return
	}

	cacheService := cache.NewCache(cfg.CacheSize, cfg.CacheTTL)

	srv := server.New(cfg, db, cacheService, log)
	
	log.Info("Starting Court Data Fetcher", 
		"host", cfg.Host,
		"port", cfg.Port,
		"court", cfg.CourtName,
	)

	if err := srv.Run(); err != nil {
		log.Fatal("Server failed to start", "error", err)
	}
}