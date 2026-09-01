package infrastructure

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/identity/domain"
	"github.com/foodapp/backend/internal/platform/middleware"
)

type JWTTokenIssuer struct {
	accessSecret   string
	issuer         string
	accessTokenTTL time.Duration
}

func NewJWTTokenIssuer(accessSecret, issuer string, accessTokenTTL time.Duration) *JWTTokenIssuer {
	return &JWTTokenIssuer{accessSecret: accessSecret, issuer: issuer, accessTokenTTL: accessTokenTTL}
}

func (j *JWTTokenIssuer) IssueAccessToken(userID uuid.UUID, role string, scopes []string) (string, int64, error) {
	now := time.Now()
	claims := middleware.Claims{
		UserID: userID.String(),
		Role:   role,
		Scopes: scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    j.issuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.accessTokenTTL)),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(j.accessSecret))
	if err != nil {
		return "", 0, err
	}
	return signed, int64(j.accessTokenTTL.Seconds()), nil
}

// GenerateRefreshToken creates a high-entropy opaque token. Only the hash
// is persisted (see HashRefreshToken) so a DB leak alone can't be used to
// forge sessions.
func (j *JWTTokenIssuer) GenerateRefreshToken() (raw string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	hash = j.HashRefreshToken(raw)
	return raw, hash, nil
}

func (j *JWTTokenIssuer) HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

var _ domain.TokenIssuer = (*JWTTokenIssuer)(nil)
