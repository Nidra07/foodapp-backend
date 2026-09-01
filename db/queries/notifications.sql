-- name: CreateNotification :one
INSERT INTO notifications (user_id, category, channel, title, body, data)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: MarkNotificationSent :exec
UPDATE notifications SET send_status = 'sent', sent_at = now() WHERE id = $1;

-- name: MarkNotificationFailed :exec
UPDATE notifications SET send_status = 'failed', failure_reason = $2 WHERE id = $1;

-- name: MarkNotificationSkipped :exec
UPDATE notifications SET send_status = 'skipped' WHERE id = $1;

-- name: ListNotificationsForUser :many
SELECT * FROM notifications
WHERE user_id = $1 AND channel = 'in_app'
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountUnreadForUser :one
SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND channel = 'in_app' AND is_read = false;

-- name: MarkNotificationRead :exec
UPDATE notifications SET is_read = true, read_at = now() WHERE id = $1 AND user_id = $2;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET is_read = true, read_at = now() WHERE user_id = $1 AND channel = 'in_app' AND is_read = false;

-- name: GetUserPreference :one
SELECT * FROM notification_preferences WHERE user_id = $1 AND category = $2 AND channel = $3;

-- name: ListUserPreferences :many
SELECT * FROM notification_preferences WHERE user_id = $1;

-- name: UpsertUserPreference :one
INSERT INTO notification_preferences (user_id, category, channel, enabled)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, category, channel) DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = now()
RETURNING *;
