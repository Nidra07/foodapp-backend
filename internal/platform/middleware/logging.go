package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/platform/logger"
)

const RequestIDHeader = "X-Request-ID"
const requestIDCtxKey = "request_id"

// RequestID assigns (or forwards) a request ID and stores it on the gin
// context so downstream handlers, error responses, and log lines all
// correlate to the same value.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(RequestIDHeader)
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Set(requestIDCtxKey, rid)
		c.Header(RequestIDHeader, rid)
		c.Next()
	}
}

func GetRequestID(c *gin.Context) string {
	if v, ok := c.Get(requestIDCtxKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// RequestLogging logs one structured line per request and injects a
// request-scoped logger into context for handlers to use.
func RequestLogging(base *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		rid := GetRequestID(c)

		ctx := base.WithContext(c.Request.Context(), map[string]interface{}{
			"request_id": rid,
			"method":     c.Request.Method,
			"path":       c.FullPath(),
		})
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		l := logger.FromContext(c.Request.Context(), base)
		latency := time.Since(start)
		status := c.Writer.Status()

		event := l.Info()
		if status >= 500 {
			event = l.Error()
		} else if status >= 400 {
			event = l.Warn()
		}
		event.
			Int("status", status).
			Dur("latency_ms", latency).
			Str("client_ip", c.ClientIP()).
			Msg("request completed")
	}
}
