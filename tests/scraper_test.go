package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/yourusername/court-data-fetcher/internal/config"
	"github.com/yourusername/court-data-fetcher/internal/scraper"
	"github.com/yourusername/court-data-fetcher/pkg/logger"
)

func TestScraperInitialization(t *testing.T) {
	// Create test config
	cfg := &config.Config{
		CourtBaseURL:   "https://districts.ecourts.gov.in/delhi",
		HeadlessMode:   true,
		ScraperTimeout: 30 * time.Second,
		UserAgent:      "Test User Agent",
	}

	// Create logger
	log, err := logger.NewLogger("debug", "text")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Initialize scraper
	s, err := scraper.NewScraper(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create scraper: %v", err)
	}
	defer s.Close()

	if s == nil {
		t.Error("Scraper should not be nil")
	}
}

func TestCaseQuery(t *testing.T) {
	tests := []struct {
		name       string
		query      scraper.CaseQuery
		wantError  bool
	}{
		{
			name: "Valid query",
			query: scraper.CaseQuery{
				CaseType:   "CS",
				CaseNumber: "1234",
				FilingYear: "2023",
			},
			wantError: false,
		},
		{
			name: "Empty case type",
			query: scraper.CaseQuery{
				CaseType:   "",
				CaseNumber: "1234",
				FilingYear: "2023",
			},
			wantError: true,
		},
		{
			name: "Empty case number",
			query: scraper.CaseQuery{
				CaseType:   "CS",
				CaseNumber: "",
				FilingYear: "2023",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate query
			err := validateQuery(tt.query)
			if (err != nil) != tt.wantError {
				t.Errorf("validateQuery() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestConcurrentScraping(t *testing.T) {
	// Skip in CI environment
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := &config.Config{
		CourtBaseURL:         "https://districts.ecourts.gov.in/delhi",
		HeadlessMode:         true,
		ScraperTimeout:       30 * time.Second,
		MaxConcurrentScrapes: 3,
		UserAgent:            "Test User Agent",
	}

	log, _ := logger.NewLogger("debug", "text")
	s, err := scraper.NewScraper(cfg, log)
	if err != nil {
		t.Fatalf("Failed to create scraper: %v", err)
	}
	defer s.Close()

	// Create multiple queries
	queries := []scraper.CaseQuery{
		{CaseType: "CS", CaseNumber: "100", FilingYear: "2023"},
		{CaseType: "CS", CaseNumber: "200", FilingYear: "2023"},
		{CaseType: "CS", CaseNumber: "300", FilingYear: "2023"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Execute concurrent search
	results, err := s.SearchCaseConcurrent(ctx, queries)
	if err != nil {
		t.Fatalf("Concurrent search failed: %v", err)
	}

	if len(results) != len(queries) {
		t.Errorf("Expected %d results, got %d", len(queries), len(results))
	}

	// Check each result
	for i, result := range results {
		if result.Query.CaseNumber != queries[i].CaseNumber {
			t.Errorf("Result %d has wrong case number: expected %s, got %s",
				i, queries[i].CaseNumber, result.Query.CaseNumber)
		}
	}
}

// Helper function to validate query
func validateQuery(q scraper.CaseQuery) error {
	if q.CaseType == "" {
		return fmt.Errorf("case type is required")
	}
	if q.CaseNumber == "" {
		return fmt.Errorf("case number is required")
	}
	if q.FilingYear == "" {
		return fmt.Errorf("filing year is required")
	}
	return nil
}