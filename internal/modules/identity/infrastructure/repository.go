// This adapter implements domain.Repository on top of sqlc-generated
// code. It is the ONLY place in the identity module that imports the
// sqlc package or knows SQL exists — application/domain code never sees
// pgx or sqlc types, only domain.* structs. Requires `sqlc generate`
// (see sqlc.yaml) to produce internal/platform/db/sqlc before this
// package compiles — see README "Generating sqlc code".
package infrastructure

import (
	"context"
	"errors"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/foodapp/backend/internal/modules/identity/domain"
	sqlcgen "github.com/foodapp/backend/internal/platform/db/sqlc"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type Repository struct {
	q *sqlcgen.Queries
}

func NewRepository(q *sqlcgen.Queries) *Repository {
	return &Repository{q: q}
}

func (r *Repository) CreateOTPChallenge(ctx context.Context, c *domain.OTPChallenge) (*domain.OTPChallenge, error) {
	row, err := r.q.CreateOTPChallenge(ctx, sqlcgen.CreateOTPChallengeParams{
		Identifier:  c.Identifier,
		Purpose:     sqlcgen.OtpPurpose(c.Purpose),
		CodeHash:    c.CodeHash,
		MaxAttempts: int16(c.MaxAttempts),
		ExpiresAt:   c.ExpiresAt,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create otp challenge", err)
	}
	return mapOTP(row), nil
}

func (r *Repository) GetLatestActiveOTP(ctx context.Context, identifier string, purpose domain.OTPPurpose) (*domain.OTPChallenge, error) {
	row, err := r.q.GetLatestActiveOTP(ctx, sqlcgen.GetLatestActiveOTPParams{
		Identifier: identifier,
		Purpose:    sqlcgen.OtpPurpose(purpose),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("otp challenge")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch otp challenge", err)
	}
	return mapOTP(row), nil
}

func (r *Repository) IncrementOTPAttempt(ctx context.Context, id uuid.UUID) (*domain.OTPChallenge, error) {
	row, err := r.q.IncrementOTPAttempt(ctx, id)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to increment otp attempt", err)
	}
	return mapOTP(row), nil
}

func (r *Repository) ConsumeOTP(ctx context.Context, id uuid.UUID) error {
	if err := r.q.ConsumeOTP(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to consume otp", err)
	}
	return nil
}

func (r *Repository) CountRecentOTPRequests(ctx context.Context, identifier string) (int64, error) {
	n, err := r.q.CountRecentOTPRequests(ctx, identifier)
	if err != nil {
		return 0, apperr.Wrap(apperr.CodeInternal, "failed to count otp requests", err)
	}
	return n, nil
}

func (r *Repository) UpsertDevice(ctx context.Context, d *domain.Device) (*domain.Device, error) {
	row, err := r.q.UpsertDevice(ctx, sqlcgen.UpsertDeviceParams{
		UserID:     d.UserID,
		DeviceID:   d.DeviceID,
		Platform:   string(d.Platform),
		FcmToken:   toText(d.FCMToken),
		AppVersion: toText(d.AppVersion),
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to upsert device", err)
	}
	return &domain.Device{
		ID:         row.ID,
		UserID:     row.UserID,
		DeviceID:   row.DeviceID,
		Platform:   domain.Platform(row.Platform),
		FCMToken:   row.FcmToken.String,
		AppVersion: row.AppVersion.String,
		LastSeenAt: row.LastSeenAt,
	}, nil
}

func (r *Repository) CreateRefreshToken(ctx context.Context, t *domain.RefreshToken) (*domain.RefreshToken, error) {
	var deviceID pgtype.UUID
	if t.DeviceID != nil {
		deviceID = pgtype.UUID{Bytes: *t.DeviceID, Valid: true}
	}

	row, err := r.q.CreateRefreshToken(ctx, sqlcgen.CreateRefreshTokenParams{
		UserID:    t.UserID,
		DeviceID:  deviceID,
		TokenHash: t.TokenHash,
		FamilyID:  t.FamilyID,
		ExpiresAt: t.ExpiresAt,
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to create refresh token", err)
	}
	return mapRefreshToken(row), nil
}

func (r *Repository) GetRefreshTokenByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	row, err := r.q.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.Unauthorized("invalid refresh token")
		}
		return nil, apperr.Wrap(apperr.CodeInternal, "failed to fetch refresh token", err)
	}
	return mapRefreshToken(row), nil
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	if err := r.q.RevokeRefreshToken(ctx, id); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to revoke refresh token", err)
	}
	return nil
}

func (r *Repository) RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error {
	if err := r.q.RevokeRefreshTokenFamily(ctx, familyID); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to revoke refresh token family", err)
	}
	return nil
}

func (r *Repository) RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	if err := r.q.RevokeAllUserRefreshTokens(ctx, userID); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to revoke user refresh tokens", err)
	}
	return nil
}

func (r *Repository) LinkRefreshTokenReplacement(ctx context.Context, oldID, newID uuid.UUID) error {
	if err := r.q.LinkRefreshTokenReplacement(ctx, sqlcgen.LinkRefreshTokenReplacementParams{
		ID:            oldID,
		ReplacedByID:  pgtype.UUID{Bytes: newID, Valid: true},
	}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to link refresh token replacement", err)
	}
	return nil
}

func (r *Repository) RecordLoginAttempt(ctx context.Context, identifier string, success bool, ip, userAgent string) error {
	if err := r.q.RecordLoginAttempt(ctx, sqlcgen.RecordLoginAttemptParams{
		Identifier: identifier,
		Success:    success,
		IpAddress:  toInet(ip),
		UserAgent:  toText(userAgent),
	}); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "failed to record login attempt", err)
	}
	return nil
}

func (r *Repository) CountRecentFailedLogins(ctx context.Context, identifier string) (int64, error) {
	n, err := r.q.CountRecentFailedLogins(ctx, identifier)
	if err != nil {
		return 0, apperr.Wrap(apperr.CodeInternal, "failed to count failed logins", err)
	}
	return n, nil
}

// --- mapping helpers ---

func mapOTP(row sqlcgen.OtpChallenge) *domain.OTPChallenge {
	var consumedAt *time.Time
	if row.ConsumedAt.Valid {
		t := row.ConsumedAt.Time
		consumedAt = &t
	}
	return &domain.OTPChallenge{
		ID:           row.ID,
		Identifier:   row.Identifier,
		Purpose:      domain.OTPPurpose(row.Purpose),
		CodeHash:     row.CodeHash,
		AttemptCount: int(row.AttemptCount),
		MaxAttempts:  int(row.MaxAttempts),
		ExpiresAt:    row.ExpiresAt,
		ConsumedAt:   consumedAt,
		CreatedAt:    row.CreatedAt,
	}
}

func mapRefreshToken(row sqlcgen.RefreshToken) *domain.RefreshToken {
	var deviceID *uuid.UUID
	if row.DeviceID.Valid {
		id := uuid.UUID(row.DeviceID.Bytes)
		deviceID = &id
	}
	var revokedAt *time.Time
	if row.RevokedAt.Valid {
		t := row.RevokedAt.Time
		revokedAt = &t
	}
	var replacedBy *uuid.UUID
	if row.ReplacedByID.Valid {
		id := uuid.UUID(row.ReplacedByID.Bytes)
		replacedBy = &id
	}
	return &domain.RefreshToken{
		ID:           row.ID,
		UserID:       row.UserID,
		DeviceID:     deviceID,
		TokenHash:    row.TokenHash,
		FamilyID:     row.FamilyID,
		IssuedAt:     row.IssuedAt,
		ExpiresAt:    row.ExpiresAt,
		RevokedAt:    revokedAt,
		ReplacedByID: replacedBy,
	}
}

func toText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: s, Valid: true}
}

func toInet(s string) pgtype.Inet {
	if s == "" {
		return pgtype.Inet{Valid: false}
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return pgtype.Inet{Valid: false}
	}
	prefix := netip.PrefixFrom(addr, addr.BitLen())
	return pgtype.Inet{Addr: prefix, Valid: true}
}

var _ domain.Repository = (*Repository)(nil)
