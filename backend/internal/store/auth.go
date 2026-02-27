package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// AdminUserRecord represents an admin user in the database.
type AdminUserRecord struct {
	ID           string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SessionRecord represents a login session in the database.
type SessionRecord struct {
	ID        string // SHA-256 hash of the session token
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// APIKeyRecord represents an API key in the database.
type APIKeyRecord struct {
	ID         string
	Name       string
	Prefix     string
	KeyHash    string
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

// AuthStore handles authentication-related persistence.
type AuthStore struct {
	db *sql.DB
}

// NewAuthStore creates a new AuthStore using the global DB connection.
func NewAuthStore() *AuthStore {
	return &AuthStore{db: DB}
}

// --- Admin methods ---

// GetAdminByUsername returns the admin user with the given username, or nil if not found.
func (s *AuthStore) GetAdminByUsername(ctx context.Context, username string) (*AdminUserRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, created_at, updated_at
		FROM admin_users WHERE username = ?
	`, username)

	var rec AdminUserRecord
	err := row.Scan(&rec.ID, &rec.Username, &rec.PasswordHash, &rec.CreatedAt, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query admin user: %w", err)
	}
	return &rec, nil
}

// GetAdminByID returns the admin user with the given ID, or nil if not found.
func (s *AuthStore) GetAdminByID(ctx context.Context, id string) (*AdminUserRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, created_at, updated_at
		FROM admin_users WHERE id = ?
	`, id)

	var rec AdminUserRecord
	err := row.Scan(&rec.ID, &rec.Username, &rec.PasswordHash, &rec.CreatedAt, &rec.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query admin user by id: %w", err)
	}
	return &rec, nil
}

// CreateAdmin inserts a new admin user record.
func (s *AuthStore) CreateAdmin(ctx context.Context, rec *AdminUserRecord) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_users (id, username, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, rec.ID, rec.Username, rec.PasswordHash, now, now)
	if err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}
	return nil
}

// UpdateAdminPassword updates the password hash for the given admin user.
func (s *AuthStore) UpdateAdminPassword(ctx context.Context, id, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE admin_users SET password_hash = ?, updated_at = ? WHERE id = ?
	`, passwordHash, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update admin password: %w", err)
	}
	return nil
}

// AdminCount returns the number of admin users in the database.
func (s *AuthStore) AdminCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count admin users: %w", err)
	}
	return count, nil
}

// --- Session methods ---

// CreateSession inserts a new session record.
func (s *AuthStore) CreateSession(ctx context.Context, rec *SessionRecord) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, expires_at, created_at)
		VALUES (?, ?, ?, ?)
	`, rec.ID, rec.UserID, rec.ExpiresAt, now)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

// GetSession returns the session with the given token hash, or nil if not found.
func (s *AuthStore) GetSession(ctx context.Context, tokenHash string) (*SessionRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, expires_at, created_at
		FROM sessions WHERE id = ?
	`, tokenHash)

	var rec SessionRecord
	err := row.Scan(&rec.ID, &rec.UserID, &rec.ExpiresAt, &rec.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query session: %w", err)
	}
	return &rec, nil
}

// DeleteSession deletes the session with the given token hash.
func (s *AuthStore) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, tokenHash)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes all sessions that have expired before the given time.
func (s *AuthStore) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired sessions: %w", err)
	}
	return result.RowsAffected()
}

// --- API Key methods ---

// CreateAPIKey inserts a new API key record.
func (s *AuthStore) CreateAPIKey(ctx context.Context, rec *APIKeyRecord) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_keys (id, name, prefix, key_hash, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, rec.ID, rec.Name, rec.Prefix, rec.KeyHash, toNullTime(rec.ExpiresAt), now)
	if err != nil {
		return fmt.Errorf("failed to create api key: %w", err)
	}
	rec.CreatedAt = now
	return nil
}

// GetAPIKeyByHash returns the API key with the given key hash, or nil if not found.
func (s *AuthStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKeyRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, prefix, key_hash, expires_at, last_used_at, created_at
		FROM api_keys WHERE key_hash = ?
	`, keyHash)

	var rec APIKeyRecord
	var expiresAt, lastUsedAt sql.NullTime
	err := row.Scan(&rec.ID, &rec.Name, &rec.Prefix, &rec.KeyHash, &expiresAt, &lastUsedAt, &rec.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query api key by hash: %w", err)
	}
	if expiresAt.Valid {
		rec.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		rec.LastUsedAt = &lastUsedAt.Time
	}
	return &rec, nil
}

// ListAPIKeys returns all API keys (without hash values).
func (s *AuthStore) ListAPIKeys(ctx context.Context) ([]APIKeyRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, prefix, expires_at, last_used_at, created_at
		FROM api_keys ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list api keys: %w", err)
	}
	defer rows.Close()

	var keys []APIKeyRecord
	for rows.Next() {
		var rec APIKeyRecord
		var expiresAt, lastUsedAt sql.NullTime
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.Prefix, &expiresAt, &lastUsedAt, &rec.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan api key: %w", err)
		}
		if expiresAt.Valid {
			rec.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			rec.LastUsedAt = &lastUsedAt.Time
		}
		keys = append(keys, rec)
	}
	return keys, rows.Err()
}

// DeleteAPIKey deletes the API key with the given ID.
func (s *AuthStore) DeleteAPIKey(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete api key: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("api key not found")
	}
	return nil
}

// UpdateAPIKeyLastUsed updates the last_used_at timestamp for the given API key.
func (s *AuthStore) UpdateAPIKeyLastUsed(ctx context.Context, id string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return fmt.Errorf("failed to update api key last_used_at: %w", err)
	}
	return nil
}
