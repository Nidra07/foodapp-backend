-- name: CreateAdminRole :one
INSERT INTO admin_roles (name, description)
VALUES ($1, $2)
RETURNING *;

-- name: GetAdminRoleByID :one
SELECT * FROM admin_roles WHERE id = $1;

-- name: GetAdminRoleByName :one
SELECT * FROM admin_roles WHERE name = $1;

-- name: ListAdminRoles :many
SELECT * FROM admin_roles ORDER BY name;

-- name: DeleteAdminRole :exec
DELETE FROM admin_roles WHERE id = $1 AND is_system = false;

-- name: ListAdminPermissions :many
SELECT * FROM admin_permissions ORDER BY code;

-- name: GetAdminPermissionByCode :one
SELECT * FROM admin_permissions WHERE code = $1;

-- name: SetRolePermissions :exec
DELETE FROM admin_role_permissions WHERE role_id = $1;

-- name: AddRolePermission :exec
INSERT INTO admin_role_permissions (role_id, permission_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: ListPermissionsForRole :many
SELECT p.* FROM admin_permissions p
JOIN admin_role_permissions rp ON rp.permission_id = p.id
WHERE rp.role_id = $1
ORDER BY p.code;

-- name: GrantAdminRole :exec
INSERT INTO admin_user_roles (user_id, role_id, granted_by) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING;

-- name: RevokeAdminRole :exec
DELETE FROM admin_user_roles WHERE user_id = $1 AND role_id = $2;

-- name: ListRolesForUser :many
SELECT r.* FROM admin_roles r
JOIN admin_user_roles ur ON ur.role_id = r.id
WHERE ur.user_id = $1
ORDER BY r.name;

-- name: ListPermissionCodesForUser :many
-- The core authorization query: every distinct permission code granted
-- to a user across all of their admin role assignments, used by
-- middleware.RequirePermission to make a single fast check.
SELECT DISTINCT p.code FROM admin_permissions p
JOIN admin_role_permissions rp ON rp.permission_id = p.id
JOIN admin_user_roles ur ON ur.role_id = rp.role_id
WHERE ur.user_id = $1;

-- name: ListAdminUsersWithRole :many
SELECT ur.user_id, ur.granted_at FROM admin_user_roles ur WHERE ur.role_id = $1 ORDER BY ur.granted_at;

-- name: CreateAuditLogEntry :one
INSERT INTO admin_audit_log (admin_user_id, action, resource_type, resource_id, details, ip_address)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListAuditLogForAdmin :many
SELECT * FROM admin_audit_log WHERE admin_user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: ListAuditLogForResource :many
SELECT * FROM admin_audit_log WHERE resource_type = $1 AND resource_id = $2 ORDER BY created_at DESC;

-- name: ListRecentAuditLog :many
SELECT * FROM admin_audit_log ORDER BY created_at DESC LIMIT $1 OFFSET $2;
