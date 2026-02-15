package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/handler"
	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/service"
	"github.com/fslongjin/liteboxd/backend/internal/store"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// Initialize SQLite database
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	dbPath := filepath.Join(dataDir, "liteboxd.db")

	fmt.Printf("Initializing database at %s\n", dbPath)
	if err := store.InitDB(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer store.CloseDB()
	fmt.Println("Database initialized")

	kubeconfigPath := os.Getenv("KUBECONFIG")
	sandboxNamespace := os.Getenv("SANDBOX_NAMESPACE")
	if sandboxNamespace == "" {
		sandboxNamespace = k8s.DefaultSandboxNamespace
	}
	controlNamespace := os.Getenv("CONTROL_NAMESPACE")
	if controlNamespace == "" {
		controlNamespace = k8s.DefaultControlNamespace
	}

	k8sClient, err := k8s.NewClient(k8s.ClientConfig{
		KubeconfigPath:   kubeconfigPath,
		SandboxNamespace: sandboxNamespace,
		ControlNamespace: controlNamespace,
	})
	if err != nil {
		log.Fatalf("Failed to create k8s client: %v", err)
	}

	ctx := context.Background()
	if err := k8sClient.EnsureNamespace(ctx); err != nil {
		log.Fatalf("Failed to ensure namespace: %v", err)
	}
	fmt.Printf("Namespace '%s' ensured\n", k8sClient.SandboxNamespace())

	// Ensure network policies are applied
	netPolicyMgr := k8s.NewNetworkPolicyManager(k8sClient)
	if err := netPolicyMgr.EnsureDefaultPolicies(ctx); err != nil {
		log.Printf("Warning: Failed to ensure network policies: %v", err)
		log.Println("Network policies may not be properly configured. Cilium may not be installed.")
	} else {
		fmt.Println("Network policies ensured")
	}

	// Create services
	templateSvc := service.NewTemplateService()
	prepullSvc := service.NewPrepullService(k8sClient)
	importExportSvc := service.NewImportExportService(templateSvc, prepullSvc)
	sandboxSvc := service.NewSandboxService(k8sClient)
	sandboxSvc.SetTemplateService(templateSvc)
	templateSvc.SetPrepullService(prepullSvc)

	sandboxSvc.StartTTLCleaner(30 * time.Second)
	fmt.Println("TTL cleaner started (interval: 30s)")

	prepullSvc.StartStatusUpdater(10 * time.Second)
	fmt.Println("Prepull status updater started (interval: 10s)")

	// Start cleanup for completed prepull DaemonSets
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			prepullSvc.CleanupCompletedPrepulls(ctx, 24*time.Hour)
		}
	}()

	// Create handlers
	sandboxHandler := handler.NewSandboxHandler(sandboxSvc)
	templateHandler := handler.NewTemplateHandler(templateSvc)
	prepullHandler := handler.NewPrepullHandler(prepullSvc, templateSvc)
	importExportHandler := handler.NewImportExportHandler(importExportSvc)

	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Upgrade", "Connection", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Extensions", "Sec-WebSocket-Protocol"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")
	sandboxHandler.RegisterRoutes(api)
	templateHandler.RegisterRoutes(api)
	prepullHandler.RegisterRoutes(api)
	importExportHandler.RegisterRoutes(api)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Server starting on port %s\n", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
