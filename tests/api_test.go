package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/court-data-fetcher/internal/api"
	"github.com/yourusername/court-data-fetcher/internal/cache"
	"github.com/yourusername/court-data-fetcher/internal/config"
	"github.com/yourusername/court-data-fetcher/internal/database"
	"github.com/yourusername/court-data-fetcher/pkg/logger"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestRouter() (*gin.Engine, *gorm.DB) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Create test database
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	database.Migrate(db)

	// Create test config
	cfg := &config.Config{
		CacheSize: 100,
		CacheTTL:  30,
	}

	// Create logger
	log, _ := logger.NewLogger("error", "json")

	// Create cache
	testCache := cache.NewCache(100, 30)

	// Create router
	router := gin.New()
	api.SetupRoutes(router, db, testCache, nil, log, cfg)

	return router, db
}

func TestHealthCheck(t *testing.T) {
	router, _ := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", response["status"])
	}
}

func TestGetCaseAPI(t *testing.T) {
	router, _ := setupTestRouter()

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{
			name:       "Valid request",
			query:      "?type=CS&number=1234&year=2023",
			wantStatus: http.StatusInternalServerError, // Will fail without scraper
		},
		{
			name:       "Missing parameters",
			query:      "?type=CS",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Empty parameters",
			query:      "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/api/case"+tt.query, nil)
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestListCasesAPI(t *testing.T) {
	router, db := setupTestRouter()

	// Insert test data
	testCase := &database.CaseInfo{
		CaseNumber: "CS/1234/2023",
		CaseType:   "CS",
		FilingYear: "2023",
		Status:     "Pending",
	}
	db.Create(testCase)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/cases?page=1&limit=10", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if !response["success"].(bool) {
		t.Error("Expected success to be true")
	}

	data := response["data"].([]interface{})
	if len(data) != 1 {
		t.Errorf("Expected 1 case, got %d", len(data))
	}
}

func TestBulkSearchAPI(t *testing.T) {
	router, _ := setupTestRouter()

	payload := map[string]interface{}{
		"queries": []map[string]string{
			{"case_type": "CS", "case_number": "100", "filing_year": "2023"},
			{"case_type": "CS", "case_number": "200", "filing_year": "2023"},
		},
	}

	jsonPayload, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/cases/bulk", bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	// Should return error without scraper, but structure should be valid
	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["success"] == nil {
		t.Error("Response should have success field")
	}
}

func TestSearchFormSubmission(t *testing.T) {
	router, _ := setupTestRouter()

	form := url.Values{}
	form.Add("case_type", "CS")
	form.Add("case_number", "1234")
	form.Add("filing_year", "2023")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/search", bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(w, req)

	// Should render error template without scraper
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestCacheStats(t *testing.T) {
	router, _ := setupTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/cache/stats", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if !response["success"].(bool) {
		t.Error("Expected success to be true")
	}

	stats := response["stats"].(map[string]interface{})
	if stats["size"] == nil {
		t.Error("Cache stats should include size")
	}
}