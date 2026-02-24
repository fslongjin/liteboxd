package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/fslongjin/liteboxd/backend/internal/security"
	"github.com/fslongjin/liteboxd/backend/internal/store"
	"github.com/gin-gonic/gin"
)

func initGatewayTestDB(t *testing.T) *store.SandboxStore {
	t.Helper()
	if err := store.InitDB(filepath.Join(t.TempDir(), "liteboxd.db")); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.CloseDB(); err != nil {
			t.Fatalf("CloseDB() error = %v", err)
		}
	})
	return store.NewSandboxStore()
}

func seedSandbox(t *testing.T, s *store.SandboxStore, sandboxID, plainToken, lifecycle string) {
	t.Helper()
	now := time.Now().UTC()
	if err := s.Create(context.Background(), &store.SandboxRecord{
		ID:                    sandboxID,
		TemplateName:          "python",
		TemplateVersion:       1,
		Image:                 "python:3.11",
		CPU:                   "500m",
		Memory:                "512Mi",
		TTL:                   3600,
		EnvJSON:               `{}`,
		DesiredState:          store.DesiredStateActive,
		LifecycleStatus:       lifecycle,
		StatusReason:          "",
		ClusterNamespace:      "liteboxd-sandbox",
		PodName:               "sandbox-" + sandboxID,
		PodUID:                "uid",
		PodPhase:              "Running",
		PodIP:                 "10.0.0.2",
		AccessTokenCiphertext: "cipher",
		AccessTokenNonce:      "nonce",
		AccessTokenKeyID:      "v1",
		AccessTokenSHA256:     security.HashToken(plainToken),
		AccessURL:             "http://gateway/api/v1/sandbox/" + sandboxID,
		CreatedAt:             now,
		ExpiresAt:             now.Add(time.Hour),
		UpdatedAt:             now,
	}); err != nil {
		t.Fatalf("seed sandbox error: %v", err)
	}
}

func TestAuthMiddlewareUsesDBTokenHash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sandboxStore := initGatewayTestDB(t)
	seedSandbox(t, sandboxStore, "abc12345", "secret-token", "running")

	svc := &Service{sandboxStore: sandboxStore}
	r := gin.New()
	r.Use(svc.AuthMiddleware())
	r.GET("/api/v1/sandbox/:sandbox/port/:port/*action", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sandbox/abc12345/port/8080/ping", nil)
	req.Header.Set(authorizationHeader, "secret-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestAuthMiddlewareRejectsInvalidOrDeletedSandbox(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sandboxStore := initGatewayTestDB(t)
	seedSandbox(t, sandboxStore, "run12345", "token-a", "running")
	seedSandbox(t, sandboxStore, "del12345", "token-b", "deleted")

	svc := &Service{sandboxStore: sandboxStore}
	r := gin.New()
	r.Use(svc.AuthMiddleware())
	r.GET("/api/v1/sandbox/:sandbox/port/:port/*action", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	cases := []struct {
		name   string
		path   string
		token  string
		status int
	}{
		{name: "invalid token", path: "/api/v1/sandbox/run12345/port/8080/ping", token: "wrong", status: http.StatusUnauthorized},
		{name: "deleted sandbox", path: "/api/v1/sandbox/del12345/port/8080/ping", token: "token-b", status: http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set(authorizationHeader, tc.token)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("expected status %d, got %d", tc.status, w.Code)
			}
		})
	}
}
