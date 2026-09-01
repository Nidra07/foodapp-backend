package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/response"
)

// PermissionChecker is the small interface this middleware depends on —
// implemented by adminrbac/application.RBACService. Defined here rather
// than imported from that package to avoid a platform -> module import,
// keeping the dependency direction consistent with the rest of the
// codebase (modules depend on platform, not the reverse).
type PermissionChecker interface {
	HasPermission(ctx context.Context, userID uuid.UUID, code string) (bool, error)
}

// RequirePermission is a finer-grained alternative to RequireRole for
// routes that need a specific admin capability (e.g. "settlements.pay")
// rather than just "any admin." Compose with RequireAuth, not as a
// replacement for RequireRole — most existing admin routes still use
// the coarser RequireRole("admin") check; this is available for NEW
// admin routes or ones deliberately retrofitted, not a blanket
// replacement — see docs/assumptions.md for exactly which routes were
// retrofitted in Phase 8.
func RequirePermission(checker PermissionChecker, code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := UserIDFromContext(c)
		if !ok {
			response.Error(c, apperr.Unauthorized(""))
			c.Abort()
			return
		}
		userUUID, err := uuid.Parse(userID)
		if err != nil {
			response.Error(c, apperr.Unauthorized(""))
			c.Abort()
			return
		}

		has, err := checker.HasPermission(c.Request.Context(), userUUID, code)
		if err != nil {
			response.Error(c, apperr.Internal(err))
			c.Abort()
			return
		}
		if !has {
			response.Error(c, apperr.Forbidden("you do not have the required permission: "+code))
			c.Abort()
			return
		}
		c.Next()
	}
}
