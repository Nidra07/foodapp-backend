package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/logger"
	"github.com/foodapp/backend/internal/platform/response"
)

// Recovery converts panics into a standardized 500 response instead of
// killing the connection, and logs the panic with a stack trace.
func Recovery(base *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				l := logger.FromContext(c.Request.Context(), base)
				l.Error().
					Interface("panic", r).
					Str("path", c.Request.URL.Path).
					Msg("panic recovered")

				response.Error(c, apperr.New(apperr.CodeInternal, "an unexpected error occurred"))
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}

// ErrorHandler lets handlers do `c.Error(err); return` and have this
// middleware translate the last recorded error into the standard envelope,
// instead of every handler calling response.Error itself.
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}
		err := c.Errors.Last().Err
		if err == nil {
			return
		}
		if !c.Writer.Written() {
			response.Error(c, err)
		}
	}
}

// NotFoundHandler handles unmatched routes with the standard envelope.
func NotFoundHandler(c *gin.Context) {
	response.Error(c, apperr.New(apperr.CodeNotFound, fmt.Sprintf("route %s %s not found", c.Request.Method, c.Request.URL.Path)))
}
