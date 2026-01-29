package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/gateway"
	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	config := gateway.LoadConfig()

	// Create k8s client
	k8sClient, err := k8s.NewClient(config.KubeconfigPath)
	if err != nil {
		log.Fatalf("Failed to create k8s client: %v", err)
	}

	// Verify k8s connection
	ctx := context.Background()
	if err := k8sClient.EnsureNamespace(ctx); err != nil {
		log.Printf("Warning: Failed to ensure namespace: %v", err)
	}

	// Create gateway service
	svc := gateway.NewService(k8sClient, config)

	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	// Create router
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	// Register routes
	svc.RegisterRoutes(r)

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + config.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Gateway server starting on port %s", config.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down gateway server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Gateway server stopped")
}
