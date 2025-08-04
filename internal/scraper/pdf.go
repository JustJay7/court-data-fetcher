package scraper

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JustJay7/court-data-fetcher/internal/database"
	"github.com/JustJay7/court-data-fetcher/pkg/logger"
	"gorm.io/gorm"
)

// PDFDownloader handles downloading and storing PDF files
type PDFDownloader struct {
	db       *gorm.DB
	logger   *logger.Logger
	savePath string
	client   *http.Client
}

// NewPDFDownloader creates a new PDF downloader
func NewPDFDownloader(db *gorm.DB, logger *logger.Logger, savePath string) *PDFDownloader {
	return &PDFDownloader{
		db:       db,
		logger:   logger,
		savePath: savePath,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// DownloadOrderPDFs downloads all PDFs for orders that haven't been downloaded yet
func (d *PDFDownloader) DownloadOrderPDFs() error {
	var orders []database.Order
	
	// Find orders with PDF links that haven't been downloaded
	if err := d.db.Where("pdf_link != ? AND downloaded = ?", "", false).Find(&orders).Error; err != nil {
		return fmt.Errorf("failed to fetch orders: %w", err)
	}

	d.logger.Info("Found orders to download", "count", len(orders))

	for _, order := range orders {
		if err := d.downloadPDF(&order); err != nil {
			d.logger.Error("Failed to download PDF", "orderID", order.ID, "error", err)
			continue
		}
		
		// Mark as downloaded
		order.Downloaded = true
		d.db.Save(&order)
		
		// Rate limiting
		time.Sleep(2 * time.Second)
	}

	return nil
}

// downloadPDF downloads a single PDF file
func (d *PDFDownloader) downloadPDF(order *database.Order) error {
	// Create directory structure: pdfs/YYYY/MM/
	now := time.Now()
	dirPath := filepath.Join(d.savePath, "pdfs", 
		fmt.Sprintf("%d", now.Year()), 
		fmt.Sprintf("%02d", now.Month()))
	
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate filename
	filename := fmt.Sprintf("order_%d_%s.pdf", 
		order.ID, 
		strings.ReplaceAll(order.OrderDate.Format("2006-01-02"), "-", ""))
	fullPath := filepath.Join(dirPath, filename)

	// Download the file
	req, err := http.NewRequest("GET", order.PDFLink, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Create the file
	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy the content
	size, err := io.Copy(file, resp.Body)
	if err != nil {
		os.Remove(fullPath) // Clean up on error
		return fmt.Errorf("failed to save file: %w", err)
	}

	// Update order with local path
	order.LocalPath = fullPath
	d.logger.Info("PDF downloaded successfully", 
		"orderID", order.ID, 
		"size", size, 
		"path", fullPath)

	return nil
}

// CleanupOldPDFs removes PDFs older than specified days
func (d *PDFDownloader) CleanupOldPDFs(daysToKeep int) error {
	cutoffDate := time.Now().AddDate(0, 0, -daysToKeep)
	
	var orders []database.Order
	if err := d.db.Where("downloaded = ? AND created_at < ? AND local_path != ?", 
		true, cutoffDate, "").Find(&orders).Error; err != nil {
		return err
	}

	for _, order := range orders {
		if err := os.Remove(order.LocalPath); err != nil {
			d.logger.Warn("Failed to remove PDF", "path", order.LocalPath, "error", err)
			continue
		}
		
		// Update database
		order.LocalPath = ""
		order.Downloaded = false
		d.db.Save(&order)
		
		d.logger.Info("Removed old PDF", "orderID", order.ID)
	}

	return nil
}