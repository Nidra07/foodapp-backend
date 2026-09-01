package middleware

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
)

// CORS is intentionally an explicit allow-list (from config), never "*",
// since the admin panel sends credentials/cookies for some flows.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" && (slices.Contains(allowedOrigins, origin) || slices.Contains(allowedOrigins, "*")) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, Idempotency-Key")
		c.Header("Access-Control-Expose-Headers", "X-Request-ID")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
