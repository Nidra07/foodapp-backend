-- name: CreateOTPChallenge :one
INSERT INTO otp_challenges (identifier, purpose, code_hash, max_attempts, expires_at, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetLatestActiveOTP :one
SELECT * FROM otp_challenges
WHERE identifier = $1 AND purpose = $2 AND consumed_at IS NULL AND expires_at > now()
ORDER BY created_at DESC
LIMIT 1;

-- name: IncrementOTPAttempt :one
UPDATE otp_challenges SET attempt_count = attempt_count + 1
WHERE id = $1
RETURNING *;

-- name: ConsumeOTP :exec
UPDATE otp_challenges SET consumed_at = now() WHERE id = $1;

-- name: CountRecentOTPRequests :one
SELECT COUNT(*) FROM otp_challenges
WHERE identifier = $1 AND created_at > now() - interval '24 hours';

-- name: UpsertDevice :one
INSERT INTO devices (user_id, device_id, platform, fcm_token, app_version, last_seen_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (user_id, device_id)
DO UPDATE SET fcm_token = EXCLUDED.fcm_token, app_version = EXCLUDED.app_version, last_seen_at = now()
RETURNING *;

-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, device_id, token_hash, family_id, expires_at, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetRefreshTokenByHash :one
SELECT * FROM refresh_tokens WHERE token_hash = $1;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1;

-- name: RevokeRefreshTokenFamily :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE family_id = $1 AND revoked_at IS NULL;

-- name: RevokeAllUserRefreshTokens :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL;

-- name: LinkRefreshTokenReplacement :exec
UPDATE refresh_tokens SET replaced_by_id = $2 WHERE id = $1;

-- name: RecordLoginAttempt :exec
INSERT INTO login_attempts (identifier, success, ip_address, user_agent)
VALUES ($1, $2, $3, $4);

-- name: CountRecentFailedLogins :one
SELECT COUNT(*) FROM login_attempts
WHERE identifier = $1 AND success = false AND created_at > now() - interval '1 hour';

-- name: ListActiveFCMTokensForUser :many
-- Used by the Notifications module (via a small DeviceLookup interface,
-- not a direct cross-module repository dependency — see
-- notifications/domain's DeviceLookup) to fan out push notifications
-- across every device a user is logged in on.
SELECT fcm_token FROM devices WHERE user_id = $1 AND fcm_token IS NOT NULL AND fcm_token != '';
