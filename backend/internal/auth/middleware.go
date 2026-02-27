package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/security"
	"github.com/fslongjin/liteboxd/backend/internal/store"
	"github.com/gin-gonic/gin"
)

const (
	SessionCookieName    = "liteboxd_session"
	APIKeyPrefix         = "lbxk_"
	ContextKeyUserID     = "auth_user_id"
	ContextKeyAuthMethod = "auth_method"

	AuthMethodSession = "session"
	AuthMethodAPIKey  = "api_key"
)

// AuthMiddleware returns a Gin middleware that authenticates requests
// via either session cookie or Bearer API key.
func AuthMiddleware(authStore *store.AuthStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try 1: Bearer token (API key)
		if authHeader := c.GetHeader("Authorization"); authHeader != "" {
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if token != "" {
					if authenticateAPIKey(c, authStore, token) {
						c.Next()
						return
					}
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
						"error": "invalid or expired API key",
					})
					return
				}
			}
		}

		// Try 2: Session cookie
		if cookie, err := c.Cookie(SessionCookieName); err == nil && cookie != "" {
			if authenticateSession(c, authStore, cookie) {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "authentication required",
		})
	}
}

// SessionOnlyMiddleware rejects requests that are not authenticated via session cookie.
// Must be used after AuthMiddleware.
func SessionOnlyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		method, exists := c.Get(ContextKeyAuthMethod)
		if !exists || method != AuthMethodSession {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "this operation requires session authentication (web login)",
			})
			return
		}
		c.Next()
	}
}

func authenticateAPIKey(c *gin.Context, authStore *store.AuthStore, token string) bool {
	keyHash := security.HashToken(token)
	apiKey, err := authStore.GetAPIKeyByHash(c.Request.Context(), keyHash)
	if err != nil || apiKey == nil {
		return false
	}
	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return false
	}
	// Note: API Key auth intentionally does not set ContextKeyUserID.
	// The api_keys table has no user association, and in the single-admin model
	// there is no need to look up the admin user for API key requests.
	// Only Me() reads ContextKeyUserID, and it already handles the missing case.
	c.Set(ContextKeyAuthMethod, AuthMethodAPIKey)
	// Update last_used_at asynchronously
	go func() {
		_ = authStore.UpdateAPIKeyLastUsed(context.Background(), apiKey.ID, time.Now())
	}()
	return true
}

func authenticateSession(c *gin.Context, authStore *store.AuthStore, token string) bool {
	tokenHash := security.HashToken(token)
	session, err := authStore.GetSession(c.Request.Context(), tokenHash)
	if err != nil || session == nil {
		return false
	}
	if session.ExpiresAt.Before(time.Now()) {
		go func() {
			_ = authStore.DeleteSession(context.Background(), tokenHash)
		}()
		return false
	}
	c.Set(ContextKeyUserID, session.UserID)
	c.Set(ContextKeyAuthMethod, AuthMethodSession)
	return true
}
