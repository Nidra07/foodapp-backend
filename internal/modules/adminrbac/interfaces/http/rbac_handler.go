package http

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/adminrbac/application"
	apperr "github.com/foodapp/backend/internal/platform/errors"
	"github.com/foodapp/backend/internal/platform/middleware"
	"github.com/foodapp/backend/internal/platform/response"
)

type RBACHandler struct {
	svc *application.RBACService
}

func NewRBACHandler(svc *application.RBACService) *RBACHandler {
	return &RBACHandler{svc: svc}
}

// RegisterRoutes mounts role/permission management under
// /admin/rbac/*, gated by the coarse admin role check plus, for the
// most sensitive actions (creating roles, assigning them), the
// fine-grained "admin.manage_roles"/"admin.assign_roles" permissions —
// this module is the one place in the codebase that eats its own dog
// food and uses RequirePermission on top of RequireRole.
func (h *RBACHandler) RegisterRoutes(rg *gin.RouterGroup, authMW, adminOnly, manageRolesMW, assignRolesMW gin.HandlerFunc) {
	roles := rg.Group("/admin/rbac/roles", authMW, adminOnly)
	roles.GET("", h.ListRoles)
	roles.POST("", manageRolesMW, h.CreateRole)
	roles.DELETE("/:id", manageRolesMW, h.DeleteRole)
	roles.PUT("/:id/permissions", manageRolesMW, h.SetRolePermissions)
	roles.GET("/:id/permissions", h.ListPermissionsForRole)

	permissions := rg.Group("/admin/rbac/permissions", authMW, adminOnly)
	permissions.GET("", h.ListPermissions)

	assignments := rg.Group("/admin/rbac/users", authMW, adminOnly)
	assignments.POST("/:userId/roles/:roleId", assignRolesMW, h.GrantRole)
	assignments.DELETE("/:userId/roles/:roleId", assignRolesMW, h.RevokeRole)
	assignments.GET("/:userId/roles", h.ListRolesForUser)

	audit := rg.Group("/admin/rbac/audit-log", authMW, adminOnly)
	audit.GET("", h.ListRecentAuditLog)
	audit.GET("/admins/:adminId", h.ListAuditLogForAdmin)
	audit.GET("/resources/:resourceType/:resourceId", h.ListAuditLogForResource)
}

type createRoleBody struct {
	Name        string  `json:"name" binding:"required"`
	Description *string `json:"description"`
}

func (h *RBACHandler) CreateRole(c *gin.Context) {
	var body createRoleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	role, err := h.svc.CreateRole(c.Request.Context(), body.Name, body.Description)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, role)
}

func (h *RBACHandler) ListRoles(c *gin.Context) {
	roles, err := h.svc.ListRoles(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, roles)
}

func (h *RBACHandler) DeleteRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid role id", nil))
		return
	}
	if err := h.svc.DeleteRole(c.Request.Context(), id); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

type setPermissionsBody struct {
	PermissionCodes []string `json:"permission_codes" binding:"required"`
}

func (h *RBACHandler) SetRolePermissions(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid role id", nil))
		return
	}
	var body setPermissionsBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, apperr.Validation("invalid request body", map[string]interface{}{"error": err.Error()}))
		return
	}
	if err := h.svc.SetRolePermissions(c.Request.Context(), id, body.PermissionCodes); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"message": "permissions updated"})
}

func (h *RBACHandler) ListPermissionsForRole(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid role id", nil))
		return
	}
	perms, err := h.svc.ListPermissionsForRole(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, perms)
}

func (h *RBACHandler) ListPermissions(c *gin.Context) {
	perms, err := h.svc.ListPermissions(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, perms)
}

func (h *RBACHandler) GrantRole(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid user id", nil))
		return
	}
	roleID, err := uuid.Parse(c.Param("roleId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid role id", nil))
		return
	}
	granterID, _ := middleware.UserIDFromContext(c)
	granterUUID, _ := uuid.Parse(granterID)

	if err := h.svc.GrantRole(c.Request.Context(), userID, roleID, granterUUID); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, gin.H{"message": "role granted"})
}

func (h *RBACHandler) RevokeRole(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid user id", nil))
		return
	}
	roleID, err := uuid.Parse(c.Param("roleId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid role id", nil))
		return
	}
	if err := h.svc.RevokeRole(c.Request.Context(), userID, roleID); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

func (h *RBACHandler) ListRolesForUser(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid user id", nil))
		return
	}
	roles, err := h.svc.ListRolesForUser(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, roles)
}

func (h *RBACHandler) ListRecentAuditLog(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))

	entries, err := h.svc.ListRecentAuditLog(c.Request.Context(), page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, entries)
}

func (h *RBACHandler) ListAuditLogForAdmin(c *gin.Context) {
	adminID, err := uuid.Parse(c.Param("adminId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid admin id", nil))
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))

	entries, err := h.svc.ListAuditLogForAdmin(c.Request.Context(), adminID, page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, entries)
}

func (h *RBACHandler) ListAuditLogForResource(c *gin.Context) {
	resourceType := c.Param("resourceType")
	resourceID, err := uuid.Parse(c.Param("resourceId"))
	if err != nil {
		response.Error(c, apperr.Validation("invalid resource id", nil))
		return
	}
	entries, err := h.svc.ListAuditLogForResource(c.Request.Context(), resourceType, resourceID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, entries)
}
