package infrastructure

import (
	"context"
	"strings"

	"github.com/foodapp/backend/internal/modules/identity/domain"
	"github.com/foodapp/backend/internal/platform/logger"
)

// MockOTPSender logs the OTP instead of sending it. Used for local/CI so
// engineers can develop the full auth flow without SMS/email provider
// credentials. NEVER wire this in for staging/production — main.go
// selects the real provider based on config.SMS.Provider / EMAIL.Provider.
type MockOTPSender struct {
	log *logger.Logger
}

func NewMockOTPSender(log *logger.Logger) *MockOTPSender {
	return &MockOTPSender{log: log}
}

func (m *MockOTPSender) SendOTP(ctx context.Context, identifier, code string, purpose domain.OTPPurpose) error {
	m.log.Info().
		Str("identifier", identifier).
		Str("purpose", string(purpose)).
		Str("otp_code", code).
		Msg("[MOCK OTP SENDER] would send OTP via SMS/email in a real environment")
	return nil
}

// CompositeOTPSender routes to SMS or email based on whether the
// identifier looks like a phone number or an email address, delegating
// to provider-specific senders that implement the same interface — this
// is the abstraction the master spec requires so no business logic is
// coupled to MSG91/Twilio/SES/SendGrid directly.
type CompositeOTPSender struct {
	SMSSender   domain.OTPSender
	EmailSender domain.OTPSender
}

func (c *CompositeOTPSender) SendOTP(ctx context.Context, identifier, code string, purpose domain.OTPPurpose) error {
	if strings.Contains(identifier, "@") {
		return c.EmailSender.SendOTP(ctx, identifier, code, purpose)
	}
	return c.SMSSender.SendOTP(ctx, identifier, code, purpose)
}

var _ domain.OTPSender = (*MockOTPSender)(nil)
var _ domain.OTPSender = (*CompositeOTPSender)(nil)
