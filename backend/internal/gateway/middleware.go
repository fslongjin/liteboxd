package gateway

import (
	"fmt"
	"net/http"

	"github.com/fslongjin/liteboxd/backend/internal/logx"
	"github.com/fslongjin/liteboxd/backend/internal/security"
	"github.com/gin-gonic/gin"
)

const (
	authorizationHeader = "X-Access-Token"
	sandboxIDParam      = "sandbox"
	portParam           = "port"
)

// AuthMiddleware creates authentication middleware for the gateway
func (s *Service) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		logger := logx.LoggerWithRequestID(c.Request.Context()).With("component", "gateway_auth")

		// Extract sandbox ID from URL path
		sandboxID := c.Param(sandboxIDParam)
		if sandboxID == "" {
			logger.Warn("missing sandbox id")
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "sandbox ID is required",
			})
			return
		}

		// Extract access token from header
		token := c.GetHeader(authorizationHeader)
		if token == "" {
			logger.Warn("missing access token", "sandbox_id", sandboxID)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing access token",
			})
			return
		}

		// Verify token against sandbox metadata in DB (source of truth).
		record, err := s.sandboxStore.GetByID(c.Request.Context(), sandboxID)
		if err != nil {
			logger.Error("failed to query sandbox metadata during auth", "sandbox_id", sandboxID, "error", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "failed to verify access token",
			})
			return
		}
		if record == nil || record.DesiredState != "active" || record.LifecycleStatus == "deleted" || record.LifecycleStatus == "lost" {
			logger.Warn("sandbox not found during auth", "sandbox_id", sandboxID)
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"error": "sandbox not found",
			})
			return
		}

		// Compare hashed token.
		if security.HashToken(token) != record.AccessTokenSHA256 {
			logger.Warn("invalid access token", "sandbox_id", sandboxID)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid access token",
			})
			return
		}

		logger.Debug("gateway auth success", "sandbox_id", sandboxID)
		// Token is valid, proceed to next handler
		c.Next()
	}
}

// CORSMiddleware adds CORS headers for WebSocket support
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, "+authorizationHeader)
		c.Header("Access-Control-Expose-Headers", "Content-Length")
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}

// ExtractPortMiddleware extracts the port from the URL path and stores it in the context
func ExtractPortMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		port := c.Param(portParam)
		if port == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "port is required",
			})
			return
		}

		// Validate port is a number
		if !isValidPort(port) {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "invalid port number",
			})
			return
		}

		c.Set("port", port)
		c.Next()
	}
}

// isValidPort checks if a port string is valid (1-65535)
func isValidPort(port string) bool {
	var p int
	_, err := fmt.Sscanf(port, "%d", &p)
	return err == nil && p >= 1 && p <= 65535
}
