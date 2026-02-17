package logx

import (
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type requestIDContextKey struct{}

func IsUUIDv4(value string) bool {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return false
	}
	return parsed.Version() == 4
}

func NormalizeRequestID(value string) string {
	if IsUUIDv4(value) {
		return value
	}
	return uuid.NewString()
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func RequestIDFromGin(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if requestID := c.GetString("request_id"); requestID != "" {
		return requestID
	}
	return RequestIDFromContext(c.Request.Context())
}

func LoggerWithRequestID(ctx context.Context) *slog.Logger {
	requestID := RequestIDFromContext(ctx)
	if requestID == "" {
		return slog.Default()
	}
	return slog.Default().With("request_id", requestID)
}
