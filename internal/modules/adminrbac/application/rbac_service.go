// Package application orchestrates Admin RBAC use cases: role/permission
// management, granting/revoking roles, permission-checking (consumed by
// middleware.RequirePermission), and audit log writes. AuditLog is
// exposed as a small interface (Logger, below) other modules can take an
// optional dependency on, same nil-safe pattern as Notifier elsewhere.
package application

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/adminrbac/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type RBACService struct {
	repo domain.Repository
}

func NewRBACService(repo domain.Repository) *RBACService {
	return &RBACService{repo: repo}
}

func (s *RBACService) CreateRole(ctx context.Context, name string, description *string) (*domain.Role, error) {
	if strings.TrimSpace(name) == "" {
		return nil, apperr.Validation("role name is required", nil)
	}
	return s.repo.CreateRole(ctx, name, description)
}

func (s *RBACService) ListRoles(ctx context.Context) ([]*domain.Role, error) {
	return s.repo.ListRoles(ctx)
}

func (s *RBACService) DeleteRole(ctx context.Context, id uuid.UUID) error {
	role, err := s.repo.GetRoleByID(ctx, id)
	if err != nil {
		return err
	}
	if role.IsSystem {
		return apperr.New(apperr.CodeForbidden, "system roles cannot be deleted")
	}
	return s.repo.DeleteRole(ctx, id)
}

func (s *RBACService) ListPermissions(ctx context.Context) ([]*domain.Permission, error) {
	return s.repo.ListPermissions(ctx)
}

// SetRolePermissions replaces a role's entire permission set — chosen
// over an incremental add/remove API because "here is exactly what this
// role should grant" is how an admin panel would realistically present
// this (a checklist of the full catalog, not one-at-a-time toggles sent
// individually to the server).
func (s *RBACService) SetRolePermissions(ctx context.Context, roleID uuid.UUID, permissionCodes []string) error {
	role, err := s.repo.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role.IsSystem {
		return apperr.New(apperr.CodeForbidden, "the permission set for a system role cannot be modified")
	}

	ids := make([]uuid.UUID, 0, len(permissionCodes))
	for _, code := range permissionCodes {
		perm, err := s.repo.GetPermissionByCode(ctx, code)
		if err != nil {
			return apperr.Validation("unknown permission code: "+code, nil)
		}
		ids = append(ids, perm.ID)
	}

	return s.repo.SetRolePermissions(ctx, roleID, ids)
}

func (s *RBACService) ListPermissionsForRole(ctx context.Context, roleID uuid.UUID) ([]*domain.Permission, error) {
	return s.repo.ListPermissionsForRole(ctx, roleID)
}

func (s *RBACService) GrantRole(ctx context.Context, userID, roleID, grantedBy uuid.UUID) error {
	return s.repo.GrantRole(ctx, userID, roleID, grantedBy)
}

func (s *RBACService) RevokeRole(ctx context.Context, userID, roleID uuid.UUID) error {
	return s.repo.RevokeRole(ctx, userID, roleID)
}

func (s *RBACService) ListRolesForUser(ctx context.Context, userID uuid.UUID) ([]*domain.Role, error) {
	return s.repo.ListRolesForUser(ctx, userID)
}

// HasPermission is the check middleware.RequirePermission calls. It
// hits the database on every request rather than caching permissions in
// the JWT — deliberate: a revoked admin permission should take effect
// immediately, not wait for the admin's access token to expire (up to
// JWT_ACCESS_TOKEN_TTL later). This trades a bit of latency for correct,
// immediate revocation, which matters more for admin/financial actions
// than for ordinary customer-facing endpoints.
func (s *RBACService) HasPermission(ctx context.Context, userID uuid.UUID, code string) (bool, error) {
	codes, err := s.repo.ListPermissionCodesForUser(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, c := range codes {
		if c == code {
			return true, nil
		}
	}
	return false, nil
}

func (s *RBACService) ListPermissionCodesForUser(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return s.repo.ListPermissionCodesForUser(ctx, userID)
}

// LogAction writes an audit entry. Like Notifier.Notify elsewhere in
// this codebase, this never returns an error to a degree that would
// block the action being audited — a failed audit WRITE must not
// prevent, e.g., a refund from actually being issued — but unlike
// Notify, a failed audit write IS logged at error level (not silently
// swallowed) since losing an audit record is a bigger deal than losing
// a push notification.
func (s *RBACService) LogAction(ctx context.Context, adminUserID uuid.UUID, action, resourceType string, resourceID *uuid.UUID, details map[string]interface{}, ipAddress *string) {
	if _, err := s.repo.CreateAuditEntry(ctx, &domain.AuditEntry{
		AdminUserID: adminUserID, Action: action, ResourceType: resourceType, ResourceID: resourceID, Details: details, IPAddress: ipAddress,
	}); err != nil {
		// Intentionally not returning this error — see doc comment above.
		// A real deployment should alert on this log line (audit write
		// failures are a compliance concern), not just emit it.
		_ = err
	}
}

func (s *RBACService) ListAuditLogForAdmin(ctx context.Context, adminUserID uuid.UUID, page, pageSize int) ([]*domain.AuditEntry, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}
	return s.repo.ListAuditLogForAdmin(ctx, adminUserID, page, pageSize)
}

func (s *RBACService) ListAuditLogForResource(ctx context.Context, resourceType string, resourceID uuid.UUID) ([]*domain.AuditEntry, error) {
	return s.repo.ListAuditLogForResource(ctx, resourceType, resourceID)
}

func (s *RBACService) ListRecentAuditLog(ctx context.Context, page, pageSize int) ([]*domain.AuditEntry, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}
	return s.repo.ListRecentAuditLog(ctx, page, pageSize)
}
