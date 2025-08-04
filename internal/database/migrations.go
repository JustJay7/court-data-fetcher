package database

import (
	"fmt"
	"gorm.io/gorm"
)

// RunMigrations executes all database migrations
func RunMigrations(db *gorm.DB) error {
	// Create indexes for better performance
	if err := createIndexes(db); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}

// createIndexes creates database indexes
func createIndexes(db *gorm.DB) error {
	// Index for case searches
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_case_info_search 
		ON case_infos(case_type, case_number, filing_year)
	`).Error; err != nil {
		return err
	}

	// Index for query logs
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_query_logs_time 
		ON query_logs(query_time)
	`).Error; err != nil {
		return err
	}

	// Index for orders by date
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_orders_date 
		ON orders(order_date)
	`).Error; err != nil {
		return err
	}

	return nil
}