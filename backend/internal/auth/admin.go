package auth

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/fslongjin/liteboxd/backend/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	AdminUsernameEnv = "ADMIN_USERNAME"
	AdminPasswordEnv = "ADMIN_PASSWORD"
	defaultAdminUser = "admin"
	defaultAdminPass = "liteboxd-admin"
	bcryptCost       = 12
)

// EnsureAdmin checks if an admin user exists. If not, creates one from env vars.
// If admin exists and ADMIN_PASSWORD is set to a non-default value, updates the password.
func EnsureAdmin(ctx context.Context, authStore *store.AuthStore) error {
	username := os.Getenv(AdminUsernameEnv)
	if username == "" {
		username = defaultAdminUser
	}
	password := os.Getenv(AdminPasswordEnv)

	count, err := authStore.AdminCount(ctx)
	if err != nil {
		return fmt.Errorf("failed to check admin users: %w", err)
	}

	if count == 0 {
		// No admin exists — create one
		if password == "" {
			password = defaultAdminPass
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
		if err != nil {
			return fmt.Errorf("failed to hash admin password: %w", err)
		}
		admin := &store.AdminUserRecord{
			ID:           uuid.NewString(),
			Username:     username,
			PasswordHash: string(hash),
		}
		if err := authStore.CreateAdmin(ctx, admin); err != nil {
			return fmt.Errorf("failed to create admin user: %w", err)
		}
		slog.Info("admin user created", "username", username)
		return nil
	}

	// Admin exists — if password env var is set and non-default, update the password
	if password != "" && password != defaultAdminPass {
		existing, err := authStore.GetAdminByUsername(ctx, username)
		if err != nil {
			return fmt.Errorf("failed to query admin user: %w", err)
		}
		if existing == nil {
			slog.Warn("admin username from env not found in database, skipping password update", "username", username)
			return nil
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
		if err != nil {
			return fmt.Errorf("failed to hash admin password: %w", err)
		}
		if err := authStore.UpdateAdminPassword(ctx, existing.ID, string(hash)); err != nil {
			return fmt.Errorf("failed to update admin password: %w", err)
		}
		slog.Info("admin password updated from environment", "username", username)
	}

	return nil
}
