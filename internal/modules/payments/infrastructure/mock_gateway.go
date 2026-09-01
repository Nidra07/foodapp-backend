package infrastructure

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/payments/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

// MockGateway simulates a payment provider for local/CI use, generating
// its own fake order IDs and accepting an HMAC signature scheme
// identical in shape to a real provider's (e.g. Razorpay signs
// `order_id|payment_id` with a shared secret) so the verification code
// path is exercised the same way it would be in production. Swap for a
// real provider (Razorpay/Stripe/PayU) by implementing the same
// domain.PaymentGateway interface — see docs/assumptions.md.
type MockGateway struct {
	secret string
}

func NewMockGateway(secret string) *MockGateway {
	if secret == "" {
		secret = "mock_gateway_secret_change_me"
	}
	return &MockGateway{secret: secret}
}

func (g *MockGateway) Name() string { return "mock" }

func (g *MockGateway) CreateOrder(ctx context.Context, amount float64, currency string, receiptID string) (string, error) {
	return "mock_order_" + uuid.NewString()[:12], nil
}

func (g *MockGateway) VerifyCallback(cb domain.CaptureCallback) error {
	expected := g.sign(cb.GatewayOrderID, cb.GatewayPaymentID)
	if !hmac.Equal([]byte(expected), []byte(cb.GatewaySignature)) {
		return apperr.New(apperr.CodeValidation, "payment signature verification failed")
	}
	return nil
}

func (g *MockGateway) Refund(ctx context.Context, gatewayPaymentID string, amount float64, reason string) (string, error) {
	return "mock_refund_" + uuid.NewString()[:12], nil
}

// Sign is exported so the mock checkout endpoint (used by the Flutter
// apps in local/dev builds, in place of a real gateway SDK) can generate
// a valid signature to complete the simulated flow end-to-end.
func (g *MockGateway) Sign(gatewayOrderID, gatewayPaymentID string) string {
	return g.sign(gatewayOrderID, gatewayPaymentID)
}

func (g *MockGateway) sign(gatewayOrderID, gatewayPaymentID string) string {
	mac := hmac.New(sha256.New, []byte(g.secret))
	mac.Write([]byte(fmt.Sprintf("%s|%s", gatewayOrderID, gatewayPaymentID)))
	return hex.EncodeToString(mac.Sum(nil))
}

var _ domain.PaymentGateway = (*MockGateway)(nil)
