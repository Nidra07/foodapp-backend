// Mock implementations of the Notifications module's sender interfaces,
// used for local/dev/CI so the full notification flow (including the
// per-channel fan-out and preference-checking logic) is exercised
// without needing real FCM/Twilio/SES credentials. Swap for real
// implementations behind the same domain.PushSender/SMSSender/EmailSender
// interfaces before production — see docs/assumptions.md.
package infrastructure

import (
	"context"

	"github.com/foodapp/backend/internal/modules/notifications/domain"
	"github.com/foodapp/backend/internal/platform/logger"
)

type MockPushSender struct{ log *logger.Logger }

func NewMockPushSender(log *logger.Logger) *MockPushSender { return &MockPushSender{log: log} }

func (m *MockPushSender) SendPush(ctx context.Context, fcmToken, title, body string, data map[string]interface{}) error {
	m.log.Info().Str("fcm_token", fcmToken).Str("title", title).Str("body", body).Msg("[MOCK PUSH] would send via FCM")
	return nil
}

type MockSMSSender struct{ log *logger.Logger }

func NewMockSMSSender(log *logger.Logger) *MockSMSSender { return &MockSMSSender{log: log} }

func (m *MockSMSSender) SendSMS(ctx context.Context, phoneNumber, body string) error {
	m.log.Info().Str("phone", phoneNumber).Str("body", body).Msg("[MOCK SMS] would send via SMS provider")
	return nil
}

type MockEmailSender struct{ log *logger.Logger }

func NewMockEmailSender(log *logger.Logger) *MockEmailSender { return &MockEmailSender{log: log} }

func (m *MockEmailSender) SendEmail(ctx context.Context, emailAddress, subject, body string) error {
	m.log.Info().Str("email", emailAddress).Str("subject", subject).Msg("[MOCK EMAIL] would send via email provider")
	return nil
}

var _ domain.PushSender = (*MockPushSender)(nil)
var _ domain.SMSSender = (*MockSMSSender)(nil)
var _ domain.EmailSender = (*MockEmailSender)(nil)
