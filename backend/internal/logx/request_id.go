package logx

import (
	"context"
	"fmt"
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

// Logger wraps *slog.Logger with fmt.Sprintf-style convenience methods.
type Logger struct {
	*slog.Logger
}

func (l *Logger) Infof(format string, args ...any) {
	l.Info(fmt.Sprintf(format, args...))
}

func (l *Logger) Warnf(format string, args ...any) {
	l.Warn(fmt.Sprintf(format, args...))
}

func (l *Logger) Errorf(format string, args ...any) {
	l.Error(fmt.Sprintf(format, args...))
}

func (l *Logger) Debugf(format string, args ...any) {
	l.Debug(fmt.Sprintf(format, args...))
}

// WithComponent returns a Logger with request_id (from context) and the given component name.
func WithComponent(ctx context.Context, component string) *Logger {
	return &Logger{LoggerWithRequestID(ctx).With("component", component)}
}
