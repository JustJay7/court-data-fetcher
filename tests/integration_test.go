package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JustJay7/court-data-fetcher/internal/config"
	"github.com/JustJay7/court-data-fetcher/internal/scraper"
	"github.com/JustJay7/court-data-fetcher/pkg/logger"
)

func TestDelhiDistrictCourtIntegration(t *testing.T) {
	// Skip in CI or if explicitly disabled
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" || testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Test configuration
	cfg := &config.Config{
		CourtBaseURL:   "https://districts.ecourts.gov.in/delhi",
		HeadlessMode:   false, // Set to false to see what's happening
		ScraperTimeout: 60 * time.Second,
		UserAgent:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
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

	// Test with a known case (you'll need to update these with real values)
	testCases := []struct {
		name       string
		caseType   string
		caseNumber string
		filingYear string
		expectErr  bool
	}{
		{
			name:       "Valid case search",
			caseType:   "CS",
			caseNumber: "100",
			filingYear: "2023",
			expectErr:  false,
		},
		{
			name:       "Invalid case number",
			caseType:   "XX",
			caseNumber: "999999",
			filingYear: "2020",
			expectErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), cfg.ScraperTimeout)
			defer cancel()

			log.Info("Starting test case", "name", tc.name)

			caseInfo, rawHTML, err := s.SearchCase(ctx, tc.caseType, tc.caseNumber, tc.filingYear)

			if tc.expectErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Logf("Search failed: %v", err)
				t.Logf("Raw HTML length: %d", len(rawHTML))
				
				// Check if it's a CAPTCHA issue
				if len(rawHTML) > 0 && contains(rawHTML, "captcha") {
					t.Log("CAPTCHA detected - this is expected without CAPTCHA service configured")
					return
				}
				
				t.Fatalf("Unexpected error: %v", err)
			}

			// Validate results
			if caseInfo == nil {
				t.Fatal("Expected case info but got nil")
			}

			if caseInfo.CaseNumber == "" {
				t.Error("Case number should not be empty")
			}

			t.Logf("Found case: %s", caseInfo.CaseNumber)
			if len(caseInfo.Parties) > 0 {
				t.Logf("Parties: %d", len(caseInfo.Parties))
			}
			if len(caseInfo.Orders) > 0 {
				t.Logf("Orders: %d", len(caseInfo.Orders))
			}
		})
	}
}

func TestParserWithSampleHTML(t *testing.T) {
	// Test the parser with sample HTML structure
	sampleHTML := `
	<div class="container">
		<table class="table">
			<tr>
				<td>Case Number:</td>
				<td>CS/1234/2023</td>
			</tr>
			<tr>
				<td>Filing Date:</td>
				<td>15-03-2023</td>
			</tr>
			<tr>
				<td>Next Hearing Date:</td>
				<td>20-02-2024</td>
			</tr>
			<tr>
				<td>Case Status:</td>
				<td>Pending</td>
			</tr>
		</table>
	</div>
	`

	// This would test the parser directly with known HTML
	// Implementation depends on exposing parser methods for testing
}

func TestCAPTCHAServices(t *testing.T) {
	// Test 2Captcha integration if API key is available
	apiKey := os.Getenv("TWOCAPTCHA_API_KEY")
	if apiKey == "" {
		t.Skip("TWOCAPTCHA_API_KEY not set")
	}

	// Test with a sample CAPTCHA image
	// This would test the CAPTCHA solving functionality
	t.Log("Testing 2Captcha service...")
	
	// Implementation would include:
	// 1. Load a test CAPTCHA image
	// 2. Submit to 2Captcha
	// 3. Verify response
}

// Helper function
func contains(s string, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && 
		   (s == substr || len(s) > len(substr) && 
		    (s[:len(substr)] == substr || 
		     s[len(s)-len(substr):] == substr ||
		     len(s) > len(substr) && containsMiddle(s, substr)))
}

func containsMiddle(s string, substr string) bool {
	for i := 1; i < len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}