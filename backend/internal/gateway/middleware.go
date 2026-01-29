package gateway

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	authorizationHeader = "X-Access-Token"
	sandboxIDParam     = "sandbox"
	portParam          = "port"
)

// AuthMiddleware creates authentication middleware for the gateway
func (s *Service) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract sandbox ID from URL path
		sandboxID := c.Param(sandboxIDParam)
		if sandboxID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "sandbox ID is required",
			})
			return
		}

		// Extract access token from header
		token := c.GetHeader(authorizationHeader)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing access token",
			})
			return
		}

		// Verify token against pod annotation
		storedToken, err := s.k8sClient.GetPodAccessToken(c.Request.Context(), sandboxID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"error": "sandbox not found",
			})
			return
		}

		// Compare tokens
		if token != storedToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid access token",
			})
			return
		}

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
