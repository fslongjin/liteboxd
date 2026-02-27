package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/auth"
	"github.com/fslongjin/liteboxd/backend/internal/logx"
	"github.com/fslongjin/liteboxd/backend/internal/security"
	"github.com/fslongjin/liteboxd/backend/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles authentication-related HTTP requests.
type AuthHandler struct {
	authStore     *store.AuthStore
	sessionMaxAge time.Duration
	cookieSecure  *bool // nil = auto-detect from request
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authStore *store.AuthStore, sessionMaxAge time.Duration, cookieSecure *bool) *AuthHandler {
	return &AuthHandler{
		authStore:     authStore,
		sessionMaxAge: sessionMaxAge,
		cookieSecure:  cookieSecure,
	}
}

func (h *AuthHandler) logger(c *gin.Context) *logx.Logger {
	return logx.WithComponent(c.Request.Context(), "auth")
}

// RegisterRoutes registers auth routes on the given router group.
// loginGroup is public (no auth required), protectedGroup requires auth middleware.
func (h *AuthHandler) RegisterRoutes(group *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	// Public
	group.POST("/login", h.Login)

	// Protected (require auth)
	protected := group.Group("")
	protected.Use(authMiddleware)
	{
		protected.GET("/me", h.Me)

		sessionOnly := protected.Group("")
		sessionOnly.Use(auth.SessionOnlyMiddleware())
		{
			sessionOnly.POST("/logout", h.Logout)
			sessionOnly.POST("/api-keys", h.CreateAPIKey)
			sessionOnly.GET("/api-keys", h.ListAPIKeys)
			sessionOnly.DELETE("/api-keys/:id", h.DeleteAPIKey)
		}
	}
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login authenticates admin user and returns a session cookie.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	admin, err := h.authStore.GetAdminByUsername(c.Request.Context(), req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if admin == nil {
		h.logger(c).Warnf("login failed: invalid credentials, username=%s", req.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		h.logger(c).Warnf("login failed: invalid credentials, username=%s", req.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// Generate session token
	token, err := security.GenerateToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate session"})
		return
	}
	tokenHash := security.HashToken(token)
	expiresAt := time.Now().Add(h.sessionMaxAge)

	session := &store.SessionRecord{
		ID:        tokenHash,
		UserID:    admin.ID,
		ExpiresAt: expiresAt,
	}
	if err := h.authStore.CreateSession(c.Request.Context(), session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	secure := h.isSecure(c)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.SessionCookieName, token, int(h.sessionMaxAge.Seconds()), "/", "", secure, true)

	h.logger(c).Infof("admin login successful, username=%s", admin.Username)

	c.JSON(http.StatusOK, gin.H{
		"message":  "login successful",
		"username": admin.Username,
	})
}

// Logout invalidates the current session.
func (h *AuthHandler) Logout(c *gin.Context) {
	cookie, err := c.Cookie(auth.SessionCookieName)
	if err == nil && cookie != "" {
		tokenHash := security.HashToken(cookie)
		_ = h.authStore.DeleteSession(c.Request.Context(), tokenHash)
	}

	secure := h.isSecure(c)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.SessionCookieName, "", -1, "/", "", secure, true)

	h.logger(c).Info("session logged out")

	c.JSON(http.StatusOK, gin.H{"message": "logout successful"})
}

// Me returns current authentication information.
func (h *AuthHandler) Me(c *gin.Context) {
	method, _ := c.Get(auth.ContextKeyAuthMethod)
	resp := gin.H{"auth_method": method}

	if userID, exists := c.Get(auth.ContextKeyUserID); exists {
		admin, err := h.authStore.GetAdminByID(c.Request.Context(), userID.(string))
		if err == nil && admin != nil {
			resp["username"] = admin.Username
		}
	}

	c.JSON(http.StatusOK, resp)
}

type createAPIKeyRequest struct {
	Name          string `json:"name" binding:"required,max=128"`
	ExpiresInDays *int   `json:"expires_in_days,omitempty"`
}

type apiKeyResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Key       string     `json:"key,omitempty"`
	Prefix    string     `json:"prefix"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type apiKeyListItem struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CreateAPIKey creates a new API key.
func (h *AuthHandler) CreateAPIKey(c *gin.Context) {
	var req createAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required and must be at most 128 characters"})
		return
	}

	rawKey, err := security.GenerateToken(32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate key"})
		return
	}
	fullKey := auth.APIKeyPrefix + rawKey
	keyHash := security.HashToken(fullKey)
	prefix := rawKey[:8]

	var expiresAt *time.Time
	if req.ExpiresInDays != nil && *req.ExpiresInDays > 0 {
		t := time.Now().Add(time.Duration(*req.ExpiresInDays) * 24 * time.Hour)
		expiresAt = &t
	}

	rec := &store.APIKeyRecord{
		ID:        uuid.NewString(),
		Name:      req.Name,
		Prefix:    prefix,
		KeyHash:   keyHash,
		ExpiresAt: expiresAt,
	}
	if err := h.authStore.CreateAPIKey(c.Request.Context(), rec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create API key"})
		return
	}

	h.logger(c).Infof("API key created, id=%s, name=%s, prefix=%s", rec.ID, rec.Name, prefix)

	c.JSON(http.StatusCreated, apiKeyResponse{
		ID:        rec.ID,
		Name:      rec.Name,
		Key:       fullKey,
		Prefix:    prefix,
		ExpiresAt: expiresAt,
		CreatedAt: rec.CreatedAt,
	})
}

// ListAPIKeys returns all API keys (without secrets).
func (h *AuthHandler) ListAPIKeys(c *gin.Context) {
	keys, err := h.authStore.ListAPIKeys(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list API keys"})
		return
	}

	items := make([]apiKeyListItem, 0, len(keys))
	for _, k := range keys {
		items = append(items, apiKeyListItem{
			ID:         k.ID,
			Name:       k.Name,
			Prefix:     k.Prefix,
			ExpiresAt:  k.ExpiresAt,
			LastUsedAt: k.LastUsedAt,
			CreatedAt:  k.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, items)
}

// DeleteAPIKey revokes an API key.
func (h *AuthHandler) DeleteAPIKey(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	if err := h.authStore.DeleteAPIKey(c.Request.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "API key not found"})
			return
		}
		h.logger(c).Warnf("failed to delete API key, id=%s, error=%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete API key"})
		return
	}

	h.logger(c).Infof("API key deleted, id=%s", id)

	c.JSON(http.StatusOK, gin.H{"message": "API key deleted"})
}

func (h *AuthHandler) isSecure(c *gin.Context) bool {
	if h.cookieSecure != nil {
		return *h.cookieSecure
	}
	return c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
}
