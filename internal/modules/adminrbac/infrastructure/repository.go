package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/foodapp/backend/internal/modules/adminrbac/domain"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type Repository struct {
	pool *pgxpool.Pool // used only to open the transaction SetRolePermissions runs in
	q    *sqlcgen.Queries
}

func NewRepository(pool *pgxpool.Pool, q *sqlcgen.Queries) *Repository {
	return &Repository{pool: pool, q: q}
}

func (r *Repository) CreateRole(ctx context.Context, name string, description *string) (*domain.Role, error) {
	row, err := r.q.CreateAdminRole(ctx, sqlcgen.CreateAdminRoleParams{Name: name, Description: toText(description)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create role", err)
	}
	return mapRole(row), nil
}

func (r *Repository) GetRoleByID(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
	row, err := r.q.GetAdminRoleByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("admin role")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch role", err)
	}
	return mapRole(row), nil
}

func (r *Repository) GetRoleByName(ctx context.Context, name string) (*domain.Role, error) {
	row, err := r.q.GetAdminRoleByName(ctx, name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("admin role")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch role", err)
	}
	return mapRole(row), nil
}

func (r *Repository) ListRoles(ctx context.Context) ([]*domain.Role, error) {
	rows, err := r.q.ListAdminRoles(ctx)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list roles", err)
	}
	out := make([]*domain.Role, len(rows))
	for i, row := range rows {
		out[i] = mapRole(row)
	}
	return out, nil
}

func (r *Repository) DeleteRole(ctx context.Context, id uuid.UUID) error {
	if err := r.q.DeleteAdminRole(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to delete role", err)
	}
	return nil
}

func (r *Repository) ListPermissions(ctx context.Context) ([]*domain.Permission, error) {
	rows, err := r.q.ListAdminPermissions(ctx)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list permissions", err)
	}
	out := make([]*domain.Permission, len(rows))
	for i, row := range rows {
		out[i] = &domain.Permission{ID: row.ID, Code: row.Code, Description: row.Description}
	}
	return out, nil
}

func (r *Repository) GetPermissionByCode(ctx context.Context, code string) (*domain.Permission, error) {
	row, err := r.q.GetAdminPermissionByCode(ctx, code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("permission")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch permission", err)
	}
	return &domain.Permission{ID: row.ID, Code: row.Code, Description: row.Description}, nil
}

// SetRolePermissions replaces a role's entire permission set inside a
// single transaction — this used to be a documented gap (delete-all
// then insert-each as separate, unwrapped statements; see
// docs/assumptions.md, Phase 8 section) where a failure partway through
// could leave a role with a partial permission set. Fixed the same way
// as orders.Create in Phase 3's cleanup: open a transaction here, derive
// a tx-scoped *sqlcgen.Queries via q.WithTx(tx).
func (r *Repository) SetRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to begin transaction", err)
	}
	defer tx.Rollback(ctx)

	qtx := r.q.WithTx(tx)

	if err := qtx.SetRolePermissions(ctx, roleID); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to clear existing permissions", err)
	}
	for _, permID := range permissionIDs {
		if err := qtx.AddRolePermission(ctx, sqlcgen.AddRolePermissionParams{RoleID: roleID, PermissionID: permID}); err != nil {
			return apperr.Wrap(apperr.CodeInternal, "failed to set role permissions", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to commit role permission update", err)
	}
	return nil
}

func (r *Repository) ListPermissionsForRole(ctx context.Context, roleID uuid.UUID) ([]*domain.Permission, error) {
	rows, err := r.q.ListPermissionsForRole(ctx, roleID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list role permissions", err)
	}
	out := make([]*domain.Permission, len(rows))
	for i, row := range rows {
		out[i] = &domain.Permission{ID: row.ID, Code: row.Code, Description: row.Description}
	}
	return out, nil
}

func (r *Repository) GrantRole(ctx context.Context, userID, roleID, grantedBy uuid.UUID) error {
	if err := r.q.GrantAdminRole(ctx, sqlcgen.GrantAdminRoleParams{UserID: userID, RoleID: roleID, GrantedBy: pgtype.UUID{Bytes: grantedBy, Valid: true}}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to grant role", err)
	}
	return nil
}

func (r *Repository) RevokeRole(ctx context.Context, userID, roleID uuid.UUID) error {
	if err := r.q.RevokeAdminRole(ctx, sqlcgen.RevokeAdminRoleParams{UserID: userID, RoleID: roleID}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to revoke role", err)
	}
	return nil
}

func (r *Repository) ListRolesForUser(ctx context.Context, userID uuid.UUID) ([]*domain.Role, error) {
	rows, err := r.q.ListRolesForUser(ctx, userID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list user roles", err)
	}
	out := make([]*domain.Role, len(rows))
	for i, row := range rows {
		out[i] = mapRole(row)
	}
	return out, nil
}

func (r *Repository) ListPermissionCodesForUser(ctx context.Context, userID uuid.UUID) ([]string, error) {
	codes, err := r.q.ListPermissionCodesForUser(ctx, userID)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list user permissions", err)
	}
	return codes, nil
}

func (r *Repository) CreateAuditEntry(ctx context.Context, e *domain.AuditEntry) (*domain.AuditEntry, error) {
	var resourceID pgtype.UUID
	if e.ResourceID != nil {
		resourceID = pgtype.UUID{Bytes: *e.ResourceID, Valid: true}
	}
	var detailsJSON []byte
	if e.Details != nil {
		var err error
		detailsJSON, err = json.Marshal(e.Details)
		if err != nil {
			return nil, apperr.Wrap(apperr.CodeInternal, "failed to marshal audit details", err)
		}
	}

	row, err := r.q.CreateAuditLogEntry(ctx, sqlcgen.CreateAuditLogEntryParams{
		AdminUserID: e.AdminUserID, Action: e.Action, ResourceType: e.ResourceType,
		ResourceID: resourceID, Details: detailsJSON, IpAddress: toInet(e.IPAddress),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to write audit log entry", err)
	}
	return mapAuditEntry(row), nil
}

func (r *Repository) ListAuditLogForAdmin(ctx context.Context, adminUserID uuid.UUID, page, pageSize int) ([]*domain.AuditEntry, error) {
	offset := (page - 1) * pageSize
	rows, err := r.q.ListAuditLogForAdmin(ctx, sqlcgen.ListAuditLogForAdminParams{AdminUserID: adminUserID, Limit: int32(pageSize), Offset: int32(offset)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list audit log", err)
	}
	out := make([]*domain.AuditEntry, len(rows))
	for i, row := range rows {
		out[i] = mapAuditEntry(row)
	}
	return out, nil
}

func (r *Repository) ListAuditLogForResource(ctx context.Context, resourceType string, resourceID uuid.UUID) ([]*domain.AuditEntry, error) {
	rows, err := r.q.ListAuditLogForResource(ctx, sqlcgen.ListAuditLogForResourceParams{ResourceType: resourceType, ResourceID: pgtype.UUID{Bytes: resourceID, Valid: true}})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list audit log", err)
	}
	out := make([]*domain.AuditEntry, len(rows))
	for i, row := range rows {
		out[i] = mapAuditEntry(row)
	}
	return out, nil
}

func (r *Repository) ListRecentAuditLog(ctx context.Context, page, pageSize int) ([]*domain.AuditEntry, error) {
	offset := (page - 1) * pageSize
	rows, err := r.q.ListRecentAuditLog(ctx, sqlcgen.ListRecentAuditLogParams{Limit: int32(pageSize), Offset: int32(offset)})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to list audit log", err)
	}
	out := make([]*domain.AuditEntry, len(rows))
	for i, row := range rows {
		out[i] = mapAuditEntry(row)
	}
	return out, nil
}

// --- mapping helpers ---

func mapRole(row sqlcgen.AdminRole) *domain.Role {
	role := &domain.Role{ID: row.ID, Name: row.Name, IsSystem: row.IsSystem, CreatedAt: row.CreatedAt}
	if row.Description.Valid {
		role.Description = &row.Description.String
	}
	return role
}

func mapAuditEntry(row sqlcgen.AdminAuditLog) *domain.AuditEntry {
	e := &domain.AuditEntry{
		ID: row.ID, AdminUserID: row.AdminUserID, Action: row.Action, ResourceType: row.ResourceType, CreatedAt: row.CreatedAt,
	}
	if row.ResourceID.Valid {
		id := uuid.UUID(row.ResourceID.Bytes)
		e.ResourceID = &id
	}
	if len(row.Details) > 0 {
		var details map[string]interface{}
		if err := json.Unmarshal(row.Details, &details); err == nil {
			e.Details = details
		}
	}
	return e
}

func toText(s *string) pgtype.Text {
	if s == nil || *s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func toInet(s *string) pgtype.Inet {
	if s == nil || *s == "" {
		return pgtype.Inet{Valid: false}
	}
	addr, err := netip.ParseAddr(*s)
	if err != nil {
		return pgtype.Inet{Valid: false}
	}
	prefix := netip.PrefixFrom(addr, addr.BitLen())
	return pgtype.Inet{Addr: prefix, Valid: true}
}

var _ domain.Repository = (*Repository)(nil)
