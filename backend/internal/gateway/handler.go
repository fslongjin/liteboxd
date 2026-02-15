package gateway

import (
	"net/http"

	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/lifecycle"
	"github.com/gin-gonic/gin"
)

// Service is the gateway service
type Service struct {
	k8sClient  *k8s.Client
	config     *Config
	drainState *lifecycle.DrainManager
}

// NewService creates a new gateway service
func NewService(k8sClient *k8s.Client, config *Config, drainState *lifecycle.DrainManager) *Service {
	return &Service{
		k8sClient:  k8sClient,
		config:     config,
		drainState: drainState,
	}
}

// RegisterRoutes registers all gateway routes
func (s *Service) RegisterRoutes(r *gin.Engine) {
	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "gateway"})
	})
	r.GET("/readyz", func(c *gin.Context) {
		if s.drainState != nil && s.drainState.IsDraining() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "draining", "service": "gateway"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "gateway"})
	})

	// Proxy routes - requires authentication
	proxyGroup := r.Group("/api/v1")
	proxyGroup.Use(CORSMiddleware())
	proxyGroup.Use(ExtractPortMiddleware())
	proxyGroup.Use(s.AuthMiddleware())

	// Sandbox access route
	// Format: /api/v1/sandbox/:sandbox/port/:port/*
	proxyGroup.GET("/sandbox/:sandbox/port/:port/*action", s.ProxyHandler)
	proxyGroup.POST("/sandbox/:sandbox/port/:port/*action", s.ProxyHandler)
	proxyGroup.PUT("/sandbox/:sandbox/port/:port/*action", s.ProxyHandler)
	proxyGroup.DELETE("/sandbox/:sandbox/port/:port/*action", s.ProxyHandler)
	proxyGroup.PATCH("/sandbox/:sandbox/port/:port/*action", s.ProxyHandler)
}
