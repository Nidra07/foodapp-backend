// Package application holds use-case orchestration for Identity & Auth:
// request OTP, verify OTP (login/signup), refresh tokens (with rotation
// + reuse detection), and logout. It depends only on domain interfaces —
// never on gin, pgx, or sqlc — so it's unit-testable with fakes.
package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/identity/domain"
	usersdomain "github.com/foodapp/backend/internal/modules/users/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type AuthConfig struct {
	OTPLength         int
	OTPTTL            time.Duration
	OTPMaxAttempts    int
	OTPResendCooldown time.Duration
	OTPMaxPerDay      int
	RefreshTokenTTL   time.Duration
}

type AuthService struct {
	repo      domain.Repository
	userRepo  usersdomain.Repository
	sender    domain.OTPSender
	issuer    domain.TokenIssuer
	cfg       AuthConfig
}

func NewAuthService(repo domain.Repository, userRepo usersdomain.Repository, sender domain.OTPSender, issuer domain.TokenIssuer, cfg AuthConfig) *AuthService {
	return &AuthService{repo: repo, userRepo: userRepo, sender: sender, issuer: issuer, cfg: cfg}
}

// RequestOTP issues and sends a fresh OTP for the given identifier +
// purpose, enforcing resend cooldown and a daily cap so a single
// identifier can't be used to spam the SMS/email provider (cost + abuse
// vector) or brute-force via unlimited fresh codes.
func (s *AuthService) RequestOTP(ctx context.Context, identifier string, purpose domain.OTPPurpose) error {
	if existing, err := s.repo.GetLatestActiveOTP(ctx, identifier, purpose); err == nil {
		if time.Since(existing.CreatedAt) < s.cfg.OTPResendCooldown {
			return apperr.New(apperr.CodeRateLimited, fmt.Sprintf("please wait before requesting another code"))
		}
	}

	count, err := s.repo.CountRecentOTPRequests(ctx, identifier)
	if err != nil {
		return err
	}
	if int(count) >= s.cfg.OTPMaxPerDay {
		return apperr.New(apperr.CodeRateLimited, "daily OTP request limit reached, please try again tomorrow")
	}

	code, err := generateNumericOTP(s.cfg.OTPLength)
	if err != nil {
		return apperr.Internal(err)
	}
	codeHash := hashOTP(code)

	challenge := &domain.OTPChallenge{
		Identifier:  identifier,
		Purpose:     purpose,
		CodeHash:    codeHash,
		MaxAttempts: s.cfg.OTPMaxAttempts,
		ExpiresAt:   time.Now().Add(s.cfg.OTPTTL),
	}
	if _, err := s.repo.CreateOTPChallenge(ctx, challenge); err != nil {
		return err
	}

	if err := s.sender.SendOTP(ctx, identifier, code, purpose); err != nil {
		return apperr.Wrap(apperr.CodeUnavailable, "failed to send verification code", err)
	}
	return nil
}

type VerifyOTPResult struct {
	User      *usersdomain.User
	Tokens    *domain.TokenPair
	IsNewUser bool
}

// VerifyOTP validates the code, and on success either logs in an existing
// user or creates a new one (signup-via-OTP is standard for consumer food
// delivery apps — there's no separate password step), then issues a
// fresh token pair.
func (s *AuthService) VerifyOTP(ctx context.Context, identifier, code string, purpose domain.OTPPurpose, role usersdomain.Role, deviceInfo *domain.Device, ip, userAgent string) (*VerifyOTPResult, error) {
	challenge, err := s.repo.GetLatestActiveOTP(ctx, identifier, purpose)
	if err != nil {
		return nil, apperr.New(apperr.CodeValidation, "no active verification code found, please request a new one")
	}

	if challenge.IsExpired() {
		return nil, apperr.New(apperr.CodeValidation, "verification code has expired, please request a new one")
	}
	if challenge.AttemptsExceeded() {
		return nil, apperr.New(apperr.CodeValidation, "too many incorrect attempts, please request a new code")
	}

	if hashOTP(code) != challenge.CodeHash {
		if _, err := s.repo.IncrementOTPAttempt(ctx, challenge.ID); err != nil {
			return nil, err
		}
		_ = s.repo.RecordLoginAttempt(ctx, identifier, false, ip, userAgent)
		return nil, apperr.New(apperr.CodeValidation, "incorrect verification code")
	}

	if err := s.repo.ConsumeOTP(ctx, challenge.ID); err != nil {
		return nil, err
	}

	isNewUser := false
	user, err := s.userRepo.GetByPhoneOrEmail(ctx, identifier)
	if err != nil {
		if !isNotFound(err) {
			return nil, err
		}
		user, err = s.userRepo.Create(ctx, &usersdomain.User{
			PhoneNumber: identifierAsPhone(identifier),
			Email:       identifierAsEmail(identifier),
			PrimaryRole: role,
		})
		if err != nil {
			return nil, err
		}
		isNewUser = true
	}

	if err := s.userRepo.MarkPhoneVerified(ctx, user.ID); err != nil {
		return nil, err
	}
	_ = s.userRepo.UpdateLastLogin(ctx, user.ID)
	_ = s.repo.RecordLoginAttempt(ctx, identifier, true, ip, userAgent)

	if deviceInfo != nil {
		deviceInfo.UserID = user.ID
		if _, err := s.repo.UpsertDevice(ctx, deviceInfo); err != nil {
			return nil, err
		}
	}

	tokens, err := s.issueTokenPair(ctx, user, deviceInfo, ip, userAgent)
	if err != nil {
		return nil, err
	}

	return &VerifyOTPResult{User: user, Tokens: tokens, IsNewUser: isNewUser}, nil
}

// RefreshTokens rotates the refresh token: it validates the presented
// token, revokes it, issues a new access+refresh pair, and links the
// rotation chain via family_id. If a token is presented that was already
// revoked (i.e. reused), the entire family is revoked — this is the
// standard "refresh token reuse detection" pattern and treats reuse as a
// signal of token theft.
func (s *AuthService) RefreshTokens(ctx context.Context, rawRefreshToken, ip, userAgent string) (*domain.TokenPair, error) {
	hash := s.issuer.HashRefreshToken(rawRefreshToken)

	existing, err := s.repo.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		return nil, apperr.Unauthorized("invalid refresh token")
	}

	if existing.RevokedAt != nil {
		// Reuse of a revoked token: assume compromise, kill the whole family.
		_ = s.repo.RevokeRefreshTokenFamily(ctx, existing.FamilyID)
		return nil, apperr.Unauthorized("refresh token has been revoked; all sessions in this chain were terminated for security")
	}
	if !existing.IsValid() {
		return nil, apperr.Unauthorized("refresh token expired")
	}

	user, err := s.userRepo.GetByID(ctx, existing.UserID)
	if err != nil {
		return nil, err
	}

	if err := s.repo.RevokeRefreshToken(ctx, existing.ID); err != nil {
		return nil, err
	}

	newRaw, newHash, err := s.issuer.GenerateRefreshToken()
	if err != nil {
		return nil, apperr.Internal(err)
	}
	newToken := &domain.RefreshToken{
		UserID:    user.ID,
		DeviceID:  existing.DeviceID,
		TokenHash: newHash,
		FamilyID:  existing.FamilyID,
		ExpiresAt: time.Now().Add(s.cfg.RefreshTokenTTL),
	}
	created, err := s.repo.CreateRefreshToken(ctx, newToken)
	if err != nil {
		return nil, err
	}
	if err := s.repo.LinkRefreshTokenReplacement(ctx, existing.ID, created.ID); err != nil {
		return nil, err
	}

	accessToken, expiresIn, err := s.issuer.IssueAccessToken(user.ID, string(user.PrimaryRole), nil)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	return &domain.TokenPair{AccessToken: accessToken, RefreshToken: newRaw, ExpiresIn: expiresIn}, nil
}

// Logout revokes a single session (this device) by refresh token hash.
func (s *AuthService) Logout(ctx context.Context, rawRefreshToken string) error {
	hash := s.issuer.HashRefreshToken(rawRefreshToken)
	existing, err := s.repo.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		return nil // already invalid/gone — logout is idempotent
	}
	return s.repo.RevokeRefreshToken(ctx, existing.ID)
}

// LogoutAllDevices revokes every active session for a user (e.g. "log out
// everywhere" security action, or triggered by password/OTP-based
// account recovery).
func (s *AuthService) LogoutAllDevices(ctx context.Context, userID uuid.UUID) error {
	return s.repo.RevokeAllUserRefreshTokens(ctx, userID)
}

func (s *AuthService) issueTokenPair(ctx context.Context, user *usersdomain.User, device *domain.Device, ip, userAgent string) (*domain.TokenPair, error) {
	accessToken, expiresIn, err := s.issuer.IssueAccessToken(user.ID, string(user.PrimaryRole), nil)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	rawRefresh, hash, err := s.issuer.GenerateRefreshToken()
	if err != nil {
		return nil, apperr.Internal(err)
	}

	var deviceID *uuid.UUID
	if device != nil {
		deviceID = &device.ID
	}

	familyID := uuid.New()
	if _, err := s.repo.CreateRefreshToken(ctx, &domain.RefreshToken{
		UserID:    user.ID,
		DeviceID:  deviceID,
		TokenHash: hash,
		FamilyID:  familyID,
		ExpiresAt: time.Now().Add(s.cfg.RefreshTokenTTL),
	}); err != nil {
		return nil, err
	}

	return &domain.TokenPair{AccessToken: accessToken, RefreshToken: rawRefresh, ExpiresIn: expiresIn}, nil
}

// --- helpers ---

func generateNumericOTP(length int) (string, error) {
	digits := make([]byte, length)
	for i := range digits {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		digits[i] = byte('0' + n.Int64())
	}
	return string(digits), nil
}

func hashOTP(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func isNotFound(err error) bool {
	ae, ok := apperr.As(err)
	return ok && ae.Code == apperr.CodeNotFound
}

func identifierAsPhone(identifier string) *string {
	if len(identifier) > 0 && identifier[0] == '+' {
		return &identifier
	}
	return nil
}

func identifierAsEmail(identifier string) *string {
	for _, c := range identifier {
		if c == '@' {
			return &identifier
		}
	}
	return nil
}
