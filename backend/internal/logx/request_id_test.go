package logx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNormalizeRequestID(t *testing.T) {
	valid := "d4f9cbf0-5b95-4efe-a542-24f55108db4f"
	if got := NormalizeRequestID(valid); got != valid {
		t.Fatalf("expected valid v4 request id to be preserved, got %q", got)
	}

	got := NormalizeRequestID("not-a-uuid")
	if got == "not-a-uuid" {
		t.Fatalf("expected invalid request id to be replaced")
	}
	if !IsUUIDv4(got) {
		t.Fatalf("expected generated request id to be uuid v4, got %q", got)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	validReqID := "5cd6f88f-fc2d-4d55-a621-d95bdb730394"
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", validReqID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("X-Request-ID"); got != validReqID {
		t.Fatalf("expected response request id %q, got %q", validReqID, got)
	}

	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("X-Request-ID", "invalid")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	got := w.Header().Get("X-Request-ID")
	if !IsUUIDv4(got) {
		t.Fatalf("expected middleware to set uuid v4, got %q", got)
	}
}
