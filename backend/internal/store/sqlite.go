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

	// Create sandboxes table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS sandboxes (
			id TEXT PRIMARY KEY,
			template_name TEXT NOT NULL,
			template_version INTEGER NOT NULL,
			image TEXT NOT NULL,
			cpu TEXT NOT NULL,
			memory TEXT NOT NULL,
			ttl INTEGER NOT NULL,
			env_json TEXT NOT NULL DEFAULT '{}',
			desired_state TEXT NOT NULL DEFAULT 'active',
			lifecycle_status TEXT NOT NULL,
			status_reason TEXT NOT NULL DEFAULT '',
			cluster_namespace TEXT NOT NULL,
			pod_name TEXT NOT NULL,
			pod_uid TEXT NOT NULL DEFAULT '',
			pod_phase TEXT NOT NULL DEFAULT '',
			pod_ip TEXT NOT NULL DEFAULT '',
			last_seen_at TIMESTAMP,
			access_token_ciphertext TEXT NOT NULL,
			access_token_nonce TEXT NOT NULL,
			access_token_key_id TEXT NOT NULL,
			access_token_sha256 TEXT NOT NULL,
			access_url TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			deleted_at TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create sandboxes table: %w", err)
	}

	sandboxIndexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_sandboxes_lifecycle_status ON sandboxes(lifecycle_status)",
		"CREATE INDEX IF NOT EXISTS idx_sandboxes_desired_state ON sandboxes(desired_state)",
		"CREATE INDEX IF NOT EXISTS idx_sandboxes_expires_at ON sandboxes(expires_at)",
		"CREATE INDEX IF NOT EXISTS idx_sandboxes_last_seen_at ON sandboxes(last_seen_at)",
		"CREATE INDEX IF NOT EXISTS idx_sandboxes_template_name ON sandboxes(template_name)",
		"CREATE INDEX IF NOT EXISTS idx_sandboxes_access_token_sha256 ON sandboxes(access_token_sha256)",
	}
	for _, idx := range sandboxIndexes {
		if _, err := DB.Exec(idx); err != nil {
			return fmt.Errorf("failed to create sandbox index: %w", err)
		}
	}

	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS sandbox_status_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sandbox_id TEXT NOT NULL,
			source TEXT NOT NULL,
			from_status TEXT NOT NULL,
			to_status TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			payload_json TEXT NOT NULL DEFAULT '{}',
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY (sandbox_id) REFERENCES sandboxes(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create sandbox_status_history table: %w", err)
	}
	if _, err := DB.Exec("CREATE INDEX IF NOT EXISTS idx_sandbox_status_history_sid_ct ON sandbox_status_history(sandbox_id, created_at DESC)"); err != nil {
		return fmt.Errorf("failed to create sandbox status history index: %w", err)
	}

	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS sandbox_reconcile_runs (
			id TEXT PRIMARY KEY,
			trigger_type TEXT NOT NULL,
			started_at TIMESTAMP NOT NULL,
			finished_at TIMESTAMP,
			total_db INTEGER NOT NULL DEFAULT 0,
			total_k8s INTEGER NOT NULL DEFAULT 0,
			drift_count INTEGER NOT NULL DEFAULT 0,
			fixed_count INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			error TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create sandbox_reconcile_runs table: %w", err)
	}

	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS sandbox_reconcile_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			sandbox_id TEXT NOT NULL,
			drift_type TEXT NOT NULL,
			action TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			FOREIGN KEY (run_id) REFERENCES sandbox_reconcile_runs(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create sandbox_reconcile_items table: %w", err)
	}
	if _, err := DB.Exec("CREATE INDEX IF NOT EXISTS idx_sandbox_reconcile_items_run_id ON sandbox_reconcile_items(run_id)"); err != nil {
		return fmt.Errorf("failed to create sandbox reconcile items index: %w", err)
	}

	// Create admin_users table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS admin_users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create admin_users table: %w", err)
	}

	// Create sessions table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES admin_users(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create sessions table: %w", err)
	}
	if _, err := DB.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)"); err != nil {
		return fmt.Errorf("failed to create sessions index: %w", err)
	}

	// Create api_keys table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			prefix TEXT NOT NULL,
			key_hash TEXT NOT NULL,
			expires_at TIMESTAMP,
			last_used_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create api_keys table: %w", err)
	}
	// Drop existing non-unique index to replace with UNIQUE index (idempotent migration)
	if _, err := DB.Exec("DROP INDEX IF EXISTS idx_api_keys_key_hash"); err != nil {
		return fmt.Errorf("failed to drop old api_keys index: %w", err)
	}
	apiKeyIndexes := []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash)",
		"CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(prefix)",
	}
	for _, idx := range apiKeyIndexes {
		if _, err := DB.Exec(idx); err != nil {
			return fmt.Errorf("failed to create api_keys index: %w", err)
		}
	}

	return nil
}
