package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/response"
)

// RateLimit implements a fixed-window counter per keyFn(c) using Redis
// INCR + EXPIRE. Fixed-window is intentionally simple for Phase 1;
// swap for a sliding-window/token-bucket (Redis Lua script) before scale
// if burst-at-window-boundary behavior becomes a problem (documented in
// docs/assumptions.md).
func RateLimit(client *redis.Client, keyPrefix string, limit int, window time.Duration, keyFn func(c *gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := fmt.Sprintf("ratelimit:%s:%s", keyPrefix, keyFn(c))

		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		count, err := client.Incr(ctx, key).Result()
		if err != nil {
			// Fail open: Redis being down should not take down the whole API,
			// but it is logged upstream via the health check / alerting.
			c.Next()
			return
		}
		if count == 1 {
			client.Expire(ctx, key, window)
		}

		if count > int64(limit) {
			response.Error(c, apperr.New(apperr.CodeRateLimited, "too many requests, please try again later"))
			c.Abort()
			return
		}
		c.Next()
	}
}

// ByIP is a common keyFn for anonymous endpoints (OTP request, login).
func ByIP(c *gin.Context) string {
	return c.ClientIP()
}
