package logx

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-ID"

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(requestIDHeader)
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set("request_id", requestID)
		c.Writer.Header().Set(requestIDHeader, requestID)
		c.Next()
	}
}

func AccessLogMiddleware(component string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		requestID, _ := c.Get("request_id")
		slog.Log(
			c.Request.Context(),
			level,
			"http request completed",
			"component", component,
			"request_id", requestID,
			"method", c.Request.Method,
			"path", c.FullPath(),
			"raw_path", c.Request.URL.Path,
			"query", c.Request.URL.RawQuery,
			"status", status,
			"latency_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
			"errors", c.Errors.String(),
		)
	}
}
