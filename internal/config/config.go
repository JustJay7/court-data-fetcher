package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	// Server settings
	Host string
	Port string

	// Database settings
	DatabasePath string

	// Logging settings
	LogLevel  string
	LogFormat string

	// Cache settings
	CacheSize int
	CacheTTL  time.Duration

	// Court settings
	CourtBaseURL string
	CourtName    string

	// Scraper settings
	ScraperTimeout time.Duration
	HeadlessMode   bool
	UserAgent      string
	BrowserPath    string

	// Concurrency settings
	MaxConcurrentScrapes int
	WorkerPoolSize       int

	// API settings
	APIRateLimit  int
	APIRateWindow time.Duration
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		// Not an error if .env doesn't exist
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("error loading .env file: %w", err)
		}
	}

	cfg := &Config{
		Host:         getEnv("HOST", "0.0.0.0"),
		Port:         getEnv("PORT", "8080"),
		DatabasePath: getEnv("DATABASE_PATH", "./data/court_cases.db"),
		LogLevel:     getEnv("LOG_LEVEL", "info"),
		LogFormat:    getEnv("LOG_FORMAT", "json"),
		CourtBaseURL: getEnv("COURT_BASE_URL", "https://districts.ecourts.gov.in/delhi"),
		CourtName:    getEnv("COURT_NAME", "Delhi District Courts"),
		UserAgent:    getEnv("USER_AGENT", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"),
		BrowserPath:  getEnv("ROD_BROWSER_PATH", ""),
	}

	// Parse integer values
	var err error
	cfg.CacheSize, err = strconv.Atoi(getEnv("CACHE_SIZE", "1000"))
	if err != nil {
		return nil, fmt.Errorf("invalid CACHE_SIZE: %w", err)
	}

	cacheTTL, err := strconv.Atoi(getEnv("CACHE_TTL", "30"))
	if err != nil {
		return nil, fmt.Errorf("invalid CACHE_TTL: %w", err)
	}
	cfg.CacheTTL = time.Duration(cacheTTL) * time.Minute

	scraperTimeout, err := strconv.Atoi(getEnv("SCRAPER_TIMEOUT", "30"))
	if err != nil {
		return nil, fmt.Errorf("invalid SCRAPER_TIMEOUT: %w", err)
	}
	cfg.ScraperTimeout = time.Duration(scraperTimeout) * time.Second

	cfg.HeadlessMode = getEnv("HEADLESS_MODE", "true") == "true"

	cfg.MaxConcurrentScrapes, err = strconv.Atoi(getEnv("MAX_CONCURRENT_SCRAPES", "5"))
	if err != nil {
		return nil, fmt.Errorf("invalid MAX_CONCURRENT_SCRAPES: %w", err)
	}

	cfg.WorkerPoolSize, err = strconv.Atoi(getEnv("WORKER_POOL_SIZE", "10"))
	if err != nil {
		return nil, fmt.Errorf("invalid WORKER_POOL_SIZE: %w", err)
	}

	cfg.APIRateLimit, err = strconv.Atoi(getEnv("API_RATE_LIMIT", "100"))
	if err != nil {
		return nil, fmt.Errorf("invalid API_RATE_LIMIT: %w", err)
	}

	apiRateWindow, err := strconv.Atoi(getEnv("API_RATE_WINDOW", "60"))
	if err != nil {
		return nil, fmt.Errorf("invalid API_RATE_WINDOW: %w", err)
	}
	cfg.APIRateWindow = time.Duration(apiRateWindow) * time.Second

	return cfg, nil
}

// getEnv returns the value of an environment variable or a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}