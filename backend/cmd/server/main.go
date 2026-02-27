package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/auth"
	"github.com/fslongjin/liteboxd/backend/internal/handler"
	"github.com/fslongjin/liteboxd/backend/internal/k8s"
	"github.com/fslongjin/liteboxd/backend/internal/lifecycle"
	"github.com/fslongjin/liteboxd/backend/internal/logx"
	"github.com/fslongjin/liteboxd/backend/internal/security"
	"github.com/fslongjin/liteboxd/backend/internal/service"
	"github.com/fslongjin/liteboxd/backend/internal/store"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	logger, closeLogger, err := logx.Init("liteboxd-server")
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() {
		if err := closeLogger(); err != nil {
			slog.Error("failed to close logger", "error", err)
		}
	}()

	stdLog := slog.NewLogLogger(logger.Handler(), slog.LevelInfo)
	log.SetFlags(0)
	log.SetOutput(stdLog.Writer())

	// Initialize SQLite database
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	dbPath := filepath.Join(dataDir, "liteboxd.db")

	slog.Info("initializing database", "component", "store", "db_path", dbPath)
	if err := store.InitDB(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer store.CloseDB()
	slog.Info("database initialized", "component", "store")

	// Initialize auth: ensure admin user exists
	authStore := store.NewAuthStore()
	if err := auth.EnsureAdmin(context.Background(), authStore); err != nil {
		log.Fatalf("Failed to ensure admin user: %v", err)
	}

	// Parse session max age
	sessionMaxAge := 24 * time.Hour
	if v := os.Getenv("SESSION_MAX_AGE"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			sessionMaxAge = time.Duration(parsed) * time.Second
		} else {
			slog.Warn("invalid SESSION_MAX_AGE, fallback to default", "value", v)
		}
	}

	// Start session cleanup goroutine
	sessionCleanupCtx, sessionCleanupCancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				deleted, err := authStore.DeleteExpiredSessions(sessionCleanupCtx, time.Now())
				if err != nil {
					slog.Warn("failed to clean expired sessions", "error", err)
				} else if deleted > 0 {
					slog.Info("cleaned expired sessions", "count", deleted)
				}
			case <-sessionCleanupCtx.Done():
				slog.Info("session cleanup goroutine stopped")
				return
			}
		}
	}()

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
	slog.Info("sandbox namespace ensured", "component", "k8s", "namespace", k8sClient.SandboxNamespace())

	// Ensure network policies are applied
	netPolicyMgr := k8s.NewNetworkPolicyManager(k8sClient)
	if err := netPolicyMgr.EnsureDefaultPolicies(ctx); err != nil {
		slog.Warn("failed to ensure network policies", "component", "k8s", "error", err)
		slog.Warn("network policies may not be properly configured", "component", "k8s")
	} else {
		slog.Info("network policies ensured", "component", "k8s")
	}

	// Create services
	templateSvc := service.NewTemplateService()
	prepullSvc := service.NewPrepullService(k8sClient)
	importExportSvc := service.NewImportExportService(templateSvc, prepullSvc)
	tokenCipher, err := security.NewTokenCipherFromEnv()
	if err != nil {
		log.Fatalf("Failed to initialize token cipher: %v", err)
	}
	sandboxStore := store.NewSandboxStore()
	sandboxSvc := service.NewSandboxService(k8sClient, sandboxStore, tokenCipher)
	reconcileSvc := service.NewSandboxReconcileService(k8sClient, sandboxStore)
	sandboxSvc.SetTemplateService(templateSvc)
	templateSvc.SetPrepullService(prepullSvc)

	sandboxSvc.StartTTLCleaner(30 * time.Second)
	slog.Info("ttl cleaner started", "component", "sandbox_service", "interval", "30s")

	retentionDays := 7
	if v := os.Getenv("SANDBOX_METADATA_RETENTION_DAYS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			retentionDays = parsed
		} else {
			slog.Warn("invalid SANDBOX_METADATA_RETENTION_DAYS, fallback to default", "value", v, "default_days", retentionDays)
		}
	}
	sandboxSvc.StartMetadataCleaner(1*time.Hour, time.Duration(retentionDays)*24*time.Hour)
	slog.Info("metadata cleaner started", "component", "sandbox_service", "interval", "1h", "retention_days", retentionDays)

	prepullSvc.StartStatusUpdater(10 * time.Second)
	slog.Info("prepull status updater started", "component", "prepull_service", "interval", "10s")
	reconcileSvc.Start(1 * time.Minute)
	slog.Info("sandbox reconciler started", "component", "sandbox_reconciler", "interval", "1m")

	go func() {
		if _, err := reconcileSvc.Run(context.Background(), "startup"); err != nil {
			slog.Warn("startup reconcile failed", "component", "sandbox_reconciler", "error", err)
		}
	}()

	// Start cleanup for completed prepull DaemonSets
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			prepullSvc.CleanupCompletedPrepulls(ctx, 24*time.Hour)
		}
	}()

	drainState := lifecycle.NewDrainManager()

	// Create handlers
	authHandler := handler.NewAuthHandler(authStore, sessionMaxAge, nil)
	sandboxHandler := handler.NewSandboxHandler(sandboxSvc, reconcileSvc, drainState)
	templateHandler := handler.NewTemplateHandler(templateSvc)
	prepullHandler := handler.NewPrepullHandler(prepullSvc, templateSvc)
	importExportHandler := handler.NewImportExportHandler(importExportSvc)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logx.RequestIDMiddleware())
	r.Use(logx.AccessLogMiddleware("api_http"))

	r.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool {
			return true // Allow all origins; cookie SameSite provides CSRF protection
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "Upgrade", "Connection", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Extensions", "Sec-WebSocket-Protocol"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	r.Use(func(c *gin.Context) {
		if drainState.IsDraining() && c.Request.URL.Path != "/health" && c.Request.URL.Path != "/readyz" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "service is draining"})
			return
		}
		c.Next()
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.GET("/readyz", func(c *gin.Context) {
		if drainState.IsDraining() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "draining"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Auth middleware
	authMiddleware := auth.AuthMiddleware(authStore)

	// Auth routes (login is public, rest require auth)
	authGroup := r.Group("/api/v1/auth")
	authHandler.RegisterRoutes(authGroup, authMiddleware)

	// Protected API routes
	api := r.Group("/api/v1")
	api.Use(authMiddleware)
	sandboxHandler.RegisterRoutes(api)
	templateHandler.RegisterRoutes(api)
	prepullHandler.RegisterRoutes(api)
	importExportHandler.RegisterRoutes(api)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	shutdownTimeout := 30 * time.Second
	if v := os.Getenv("SHUTDOWN_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			shutdownTimeout = d
		}
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("api server starting", "component", "http_server", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down API server...")

	sessionCleanupCancel()

	drainState.StartDraining()
	time.Sleep(2 * time.Second)

	ctxShutdown, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctxShutdown); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	drainCtx, drainCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer drainCancel()
	if err := drainState.WaitWebSockets(drainCtx); err != nil {
		log.Printf("API drained with timeout, remaining active websockets: %d", drainState.ActiveWebSockets())
	}

	log.Println("API server stopped")
}
