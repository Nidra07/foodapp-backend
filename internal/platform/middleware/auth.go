package middleware

import (
	"context"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/response"
)

type ctxKey string

const (
	CtxUserID ctxKey = "user_id"
	CtxRole   ctxKey = "role"
	CtxScopes ctxKey = "scopes"
)

type Claims struct {
	UserID string   `json:"sub"`
	Role   string   `json:"role"`
	Scopes []string `json:"scopes"`
	jwt.RegisteredClaims
}

// RequireAuth validates the access token and injects identity into the
// request context. It does not itself check role/permissions — compose
// with RequireRole for that — so it stays reusable across every module.
func RequireAuth(accessSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			response.Error(c, apperr.Unauthorized("missing or malformed Authorization header"))
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, apperr.Unauthorized("unexpected signing method")
			}
			return []byte(accessSecret), nil
		})
		if err != nil || !token.Valid {
			response.Error(c, apperr.Unauthorized("invalid or expired token"))
			c.Abort()
			return
		}

		if _, err := uuid.Parse(claims.UserID); err != nil {
			response.Error(c, apperr.Unauthorized("invalid token subject"))
			c.Abort()
			return
		}

		ctx := context.WithValue(c.Request.Context(), CtxUserID, claims.UserID)
		ctx = context.WithValue(ctx, CtxRole, claims.Role)
		ctx = context.WithValue(ctx, CtxScopes, claims.Scopes)
		c.Request = c.Request.WithContext(ctx)
		c.Set(string(CtxUserID), claims.UserID)
		c.Set(string(CtxRole), claims.Role)

		c.Next()
	}
}

// RequireRole restricts a route to one of the given roles. Role is a
// coarse gate (customer/restaurant_owner/delivery_partner/admin/...);
// fine-grained permission checks (admin RBAC module) layer on top of this.
func RequireRole(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get(string(CtxRole))
		roleStr, _ := role.(string)

		if !slices.Contains(allowed, roleStr) {
			response.Error(c, apperr.Forbidden("you do not have permission to access this resource"))
			c.Abort()
			return
		}
		c.Next()
	}
}

func UserIDFromContext(c *gin.Context) (string, bool) {
	v, ok := c.Get(string(CtxUserID))
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// OptionalAuth attaches identity to the request context WHEN a valid
// bearer token is present, but never rejects the request if one isn't —
// for routes (like Search) that should work for both browsing
// (unauthenticated) and logged-in customers, with only the latter
// getting their search history attributed to their account. Any
// malformed/expired/invalid token is treated the same as no token at
// all here — silently proceeding unauthenticated — rather than
// rejecting, since a route mounting this middleware has already opted
// into "auth is optional," and surfacing a 401 for a stale token on an
// otherwise-public route would be a confusing UX regression.
func OptionalAuth(accessSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.Next()
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, apperr.Unauthorized("unexpected signing method")
			}
			return []byte(accessSecret), nil
		})
		if err != nil || !token.Valid {
			c.Next() // invalid token on an optional-auth route: proceed unauthenticated, don't reject
			return
		}
		if _, err := uuid.Parse(claims.UserID); err != nil {
			c.Next()
			return
		}

		ctx := context.WithValue(c.Request.Context(), CtxUserID, claims.UserID)
		ctx = context.WithValue(ctx, CtxRole, claims.Role)
		ctx = context.WithValue(ctx, CtxScopes, claims.Scopes)
		c.Request = c.Request.WithContext(ctx)
		c.Set(string(CtxUserID), claims.UserID)
		c.Set(string(CtxRole), claims.Role)

		c.Next()
	}
}
