package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// DB is the global database connection
var DB *sql.DB

// InitDB initializes the SQLite database connection and creates tables
func InitDB(dbPath string) error {
	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	var err error
	DB, err = sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Create tables
	if err := createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

// CloseDB closes the database connection
func CloseDB() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

func createTables() error {
	// Create templates table
	_, err := DB.Exec(`
		CREATE TABLE IF NOT EXISTS templates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			display_name TEXT,
			description TEXT,
			tags TEXT DEFAULT '[]',
			author TEXT DEFAULT '',
			is_public BOOLEAN DEFAULT 1,
			latest_version INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create templates table: %w", err)
	}

	// Create template_versions table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS template_versions (
			id TEXT PRIMARY KEY,
			template_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			spec TEXT NOT NULL,
			changelog TEXT DEFAULT '',
			created_by TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (template_id) REFERENCES templates(id) ON DELETE CASCADE,
			UNIQUE(template_id, version)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create template_versions table: %w", err)
	}

	// Create indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_templates_name ON templates(name)",
		"CREATE INDEX IF NOT EXISTS idx_templates_is_public ON templates(is_public)",
		"CREATE INDEX IF NOT EXISTS idx_templates_created_at ON templates(created_at)",
		"CREATE INDEX IF NOT EXISTS idx_template_versions_template_id ON template_versions(template_id)",
		"CREATE INDEX IF NOT EXISTS idx_template_versions_version ON template_versions(template_id, version DESC)",
	}

	for _, idx := range indexes {
		if _, err := DB.Exec(idx); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	// Create image_prepulls table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS image_prepulls (
			id TEXT PRIMARY KEY,
			image TEXT NOT NULL,
			image_hash TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			ready_nodes INTEGER DEFAULT 0,
			total_nodes INTEGER DEFAULT 0,
			error TEXT DEFAULT '',
			template TEXT DEFAULT '',
			started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			completed_at TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create image_prepulls table: %w", err)
	}

	// Create prepull indexes
	prepullIndexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_prepulls_image ON image_prepulls(image)",
		"CREATE INDEX IF NOT EXISTS idx_prepulls_status ON image_prepulls(status)",
		"CREATE INDEX IF NOT EXISTS idx_prepulls_image_hash ON image_prepulls(image_hash)",
	}

	for _, idx := range prepullIndexes {
		if _, err := DB.Exec(idx); err != nil {
			return fmt.Errorf("failed to create prepull index: %w", err)
		}
	}

	return nil
}
