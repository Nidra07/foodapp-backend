-- name: CreateUser :one
INSERT INTO users (phone_number, email, full_name, primary_role, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserByPhone :one
SELECT * FROM users WHERE phone_number = $1 AND deleted_at IS NULL;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 AND deleted_at IS NULL;

-- name: UpdateUserProfile :one
UPDATE users
SET full_name = COALESCE(sqlc.narg('full_name'), full_name),
    email = COALESCE(sqlc.narg('email'), email),
    profile_image_url = COALESCE(sqlc.narg('profile_image_url'), profile_image_url)
WHERE id = sqlc.arg('id') AND deleted_at IS NULL
RETURNING *;

-- name: MarkPhoneVerified :exec
UPDATE users SET phone_verified_at = now(), status = 'active' WHERE id = $1;

-- name: MarkEmailVerified :exec
UPDATE users SET email_verified_at = now() WHERE id = $1;

-- name: UpdateLastLogin :exec
UPDATE users SET last_login_at = now() WHERE id = $1;

-- name: SetUserStatus :exec
UPDATE users SET status = $2 WHERE id = $1;

-- name: SoftDeleteUser :exec
UPDATE users SET deleted_at = now(), status = 'deactivated' WHERE id = $1;

-- name: ListUsersByRole :many
SELECT * FROM users
WHERE primary_role = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountUsersByRole :one
SELECT COUNT(*) FROM users WHERE primary_role = $1 AND deleted_at IS NULL;

-- name: GrantUserRole :one
INSERT INTO user_roles (user_id, role)
VALUES ($1, $2)
ON CONFLICT (user_id, role) DO UPDATE SET revoked_at = NULL
RETURNING *;

-- name: ListUserRoles :many
SELECT * FROM user_roles WHERE user_id = $1 AND revoked_at IS NULL;
