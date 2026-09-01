// Package domain holds pure business types and interfaces for the
// Identity & Authentication module — no HTTP, no SQL, no framework
// imports. Application services depend on these interfaces;
// infrastructure implements them. This is the inversion the whole
// codebase follows (see docs/architecture.md "Clean Architecture").
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type OTPPurpose string

const (
	OTPPurposeLogin                OTPPurpose = "login"
	OTPPurposeSignup               OTPPurpose = "signup"
	OTPPurposePhoneVerification    OTPPurpose = "phone_verification"
	OTPPurposeEmailVerification    OTPPurpose = "email_verification"
	OTPPurposePasswordReset        OTPPurpose = "password_reset"
	OTPPurposeDeliveryConfirmation OTPPurpose = "delivery_confirmation"
)

type Platform string

const (
	PlatformIOS     Platform = "ios"
	PlatformAndroid Platform = "android"
	PlatformWeb     Platform = "web"
)

type OTPChallenge struct {
	ID           uuid.UUID
	Identifier   string
	Purpose      OTPPurpose
	CodeHash     string
	AttemptCount int
	MaxAttempts  int
	ExpiresAt    time.Time
	ConsumedAt   *time.Time
	CreatedAt    time.Time
}

func (o *OTPChallenge) IsExpired() bool     { return time.Now().After(o.ExpiresAt) }
func (o *OTPChallenge) IsConsumed() bool    { return o.ConsumedAt != nil }
func (o *OTPChallenge) AttemptsExceeded() bool { return o.AttemptCount >= o.MaxAttempts }

type Device struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	DeviceID    string
	Platform    Platform
	FCMToken    string
	AppVersion  string
	LastSeenAt  time.Time
}

type RefreshToken struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	DeviceID      *uuid.UUID
	TokenHash     string
	FamilyID      uuid.UUID
	IssuedAt      time.Time
	ExpiresAt     time.Time
	RevokedAt     *time.Time
	ReplacedByID  *uuid.UUID
}

func (r *RefreshToken) IsValid() bool {
	return r.RevokedAt == nil && time.Now().Before(r.ExpiresAt)
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64 // seconds
}

// Repository is the persistence port for this module. The infrastructure
// package provides a sqlc-backed implementation; tests can provide a fake.
type Repository interface {
	CreateOTPChallenge(ctx context.Context, c *OTPChallenge) (*OTPChallenge, error)
	GetLatestActiveOTP(ctx context.Context, identifier string, purpose OTPPurpose) (*OTPChallenge, error)
	IncrementOTPAttempt(ctx context.Context, id uuid.UUID) (*OTPChallenge, error)
	ConsumeOTP(ctx context.Context, id uuid.UUID) error
	CountRecentOTPRequests(ctx context.Context, identifier string) (int64, error)

	UpsertDevice(ctx context.Context, d *Device) (*Device, error)

	CreateRefreshToken(ctx context.Context, t *RefreshToken) (*RefreshToken, error)
	GetRefreshTokenByHash(ctx context.Context, hash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id uuid.UUID) error
	RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error
	RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID) error
	LinkRefreshTokenReplacement(ctx context.Context, oldID, newID uuid.UUID) error

	RecordLoginAttempt(ctx context.Context, identifier string, success bool, ip, userAgent string) error
	CountRecentFailedLogins(ctx context.Context, identifier string) (int64, error)
}

// OTPSender abstracts SMS/email delivery so the domain never depends on
// a specific provider (MSG91, Twilio, SES, SendGrid, or a local mock).
type OTPSender interface {
	SendOTP(ctx context.Context, identifier string, code string, purpose OTPPurpose) error
}

// TokenIssuer abstracts JWT signing so the service layer stays testable
// without a real signing key.
type TokenIssuer interface {
	IssueAccessToken(userID uuid.UUID, role string, scopes []string) (string, int64, error)
	HashRefreshToken(raw string) string
	GenerateRefreshToken() (raw string, hash string, err error)
}
