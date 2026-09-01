package http

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/users/application"
	"github.com/foodapp/backend/internal/modules/users/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/middleware"
	"github.com/foodapp/backend/internal/platform/response"
)

type UserHandler struct {
	svc *application.UserService
}

func NewUserHandler(svc *application.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// RegisterRoutes mounts both the "me" (self-service) routes and the
// admin user-management routes. authMiddleware/adminOnly are injected
// from main.go so this package doesn't need to know JWT secrets.
func (h *UserHandler) RegisterRoutes(rg *gin.RouterGroup, authMiddleware gin.HandlerFunc, adminOnly gin.HandlerFunc) {
	me := rg.Group("/users/me", authMiddleware)
	me.GET("", h.GetMe)
	me.PATCH("", h.UpdateMe)
	me.DELETE("", h.DeactivateMe)

	admin := rg.Group("/admin/users", authMiddleware, adminOnly)
	admin.GET("", h.AdminListUsers)
	admin.PATCH("/:id/status", h.AdminSetStatus)
}

func (h *UserHandler) GetMe(c *gin.Context) {
	userID, _ := middleware.UserIDFromContext(c)
	id, err := uuid.Parse(userID)
	if err != nil {
		response.Error(c, apperr.Unauthorized(""))
		return
	}

	user, err := h.svc.GetProfile(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, userToJSON(user))
}

type updateMeBody struct {
	FullName        *string `json:"full_name"`
	Email           *string `json:"email" binding:"omitempty,email"`
	ProfileImageURL *string `json:"profile_image_url"`
}

func (h *UserHandler) UpdateMe(c *gin.Context) {
	userID, _ := middleware.UserIDFromContext(c)
	id, err := uuid.Parse(userID)
	if err != nil {
		response.Error(c, apperr.Unauthorized(""))
		return
	}

	var body updateMeBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	user, err := h.svc.UpdateProfile(c.Request.Context(), id, domain.UpdateProfileInput{
		FullName:        body.FullName,
		Email:           body.Email,
		ProfileImageURL: body.ProfileImageURL,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, userToJSON(user))
}

func (h *UserHandler) DeactivateMe(c *gin.Context) {
	userID, _ := middleware.UserIDFromContext(c)
	id, err := uuid.Parse(userID)
	if err != nil {
		response.Error(c, apperr.Unauthorized(""))
		return
	}

	if err := h.svc.DeactivateAccount(c.Request.Context(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

func (h *UserHandler) AdminListUsers(c *gin.Context) {
	role := c.Query("role")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	users, total, err := h.svc.AdminListUsers(c.Request.Context(), domain.ListUsersFilter{
		Role:     domain.Role(role),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	out := make([]gin.H, len(users))
	for i, u := range users {
		out[i] = userToJSON(u)
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	response.Paginated(c, out, response.Meta{
		Page: page, PageSize: pageSize, TotalCount: total, TotalPages: totalPages,
	})
}

type setStatusBody struct {
	Status string `json:"status" binding:"required,oneof=active suspended deactivated pending_verification"`
}

func (h *UserHandler) AdminSetStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid user id", nil))
		return
	}

	var body setStatusBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}

	if err := h.svc.AdminSetStatus(c.Request.Context(), id, domain.Status(body.Status)); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

func userToJSON(u *domain.User) gin.H {
	return gin.H{
		"id":                u.ID,
		"phone_number":      u.PhoneNumber,
		"email":             u.Email,
		"full_name":         u.FullName,
		"role":              u.PrimaryRole,
		"status":            u.Status,
		"profile_image_url": u.ProfileImageURL,
		"phone_verified":    u.PhoneVerifiedAt != nil,
		"email_verified":    u.EmailVerifiedAt != nil,
		"created_at":        u.CreatedAt,
	}
}
