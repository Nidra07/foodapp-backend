package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Role struct {
	ID          uuid.UUID
	Name        string
	Description *string
	IsSystem    bool
	CreatedAt   time.Time
}

type Permission struct {
	ID          uuid.UUID
	Code        string
	Description string
}

type AuditEntry struct {
	ID           uuid.UUID
	AdminUserID  uuid.UUID
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	Details      map[string]interface{}
	IPAddress    *string
	CreatedAt    time.Time
}

// Repository is the persistence port for the Admin RBAC module.
type Repository interface {
	CreateRole(ctx context.Context, name string, description *string) (*Role, error)
	GetRoleByID(ctx context.Context, id uuid.UUID) (*Role, error)
	GetRoleByName(ctx context.Context, name string) (*Role, error)
	ListRoles(ctx context.Context) ([]*Role, error)
	DeleteRole(ctx context.Context, id uuid.UUID) error

	ListPermissions(ctx context.Context) ([]*Permission, error)
	GetPermissionByCode(ctx context.Context, code string) (*Permission, error)

	SetRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
	ListPermissionsForRole(ctx context.Context, roleID uuid.UUID) ([]*Permission, error)

	GrantRole(ctx context.Context, userID, roleID, grantedBy uuid.UUID) error
	RevokeRole(ctx context.Context, userID, roleID uuid.UUID) error
	ListRolesForUser(ctx context.Context, userID uuid.UUID) ([]*Role, error)
	ListPermissionCodesForUser(ctx context.Context, userID uuid.UUID) ([]string, error)

	CreateAuditEntry(ctx context.Context, e *AuditEntry) (*AuditEntry, error)
	ListAuditLogForAdmin(ctx context.Context, adminUserID uuid.UUID, page, pageSize int) ([]*AuditEntry, error)
	ListAuditLogForResource(ctx context.Context, resourceType string, resourceID uuid.UUID) ([]*AuditEntry, error)
	ListRecentAuditLog(ctx context.Context, page, pageSize int) ([]*AuditEntry, error)
}
