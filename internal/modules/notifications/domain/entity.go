package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Category string

const (
	CategoryOrderPlaced             Category = "order_placed"
	CategoryOrderConfirmed          Category = "order_confirmed"
	CategoryOrderPreparing          Category = "order_preparing"
	CategoryOrderReady              Category = "order_ready"
	CategoryOrderOutForDelivery     Category = "order_out_for_delivery"
	CategoryOrderDelivered          Category = "order_delivered"
	CategoryOrderCancelled          Category = "order_cancelled"
	CategoryPaymentCaptured         Category = "payment_captured"
	CategoryPaymentFailed           Category = "payment_failed"
	CategoryRefundProcessed         Category = "refund_processed"
	CategoryDeliveryAssigned        Category = "delivery_assigned"
	CategoryDeliveryPartnerArriving Category = "delivery_partner_arriving"
	CategoryPromotional             Category = "promotional"
	CategoryAccountSecurity         Category = "account_security"
)

type Channel string

const (
	ChannelPush  Channel = "push"
	ChannelSMS   Channel = "sms"
	ChannelEmail Channel = "email"
	ChannelInApp Channel = "in_app"
)

type SendStatus string

const (
	SendPending SendStatus = "pending"
	SendSent    SendStatus = "sent"
	SendFailed  SendStatus = "failed"
	SendSkipped SendStatus = "skipped"
)

type Notification struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Category      Category
	Channel       Channel
	Title         string
	Body          string
	Data          map[string]interface{}
	SendStatus    SendStatus
	FailureReason *string
	IsRead        bool
	ReadAt        *time.Time
	CreatedAt     time.Time
	SentAt        *time.Time
}

type Preference struct {
	UserID   uuid.UUID
	Category Category
	Channel  Channel
	Enabled  bool
}

// NotifyInput describes one logical event to notify a user about. The
// application layer fans this out across every channel the category is
// configured to use (see NotificationService.defaultChannelsFor) minus
// whatever the user has explicitly disabled.
type NotifyInput struct {
	UserID   uuid.UUID
	Category Category
	Title    string
	Body     string
	Data     map[string]interface{}
}

// PushSender, SMSSender, and EmailSender mirror the same
// provider-abstraction pattern used elsewhere (domain.OTPSender in
// Identity, domain.PaymentGateway in Payments): application code never
// imports FCM/Twilio/SES SDKs directly, only these interfaces. Each has
// a mock implementation for local/dev.
type PushSender interface {
	SendPush(ctx context.Context, fcmToken, title, body string, data map[string]interface{}) error
}

type SMSSender interface {
	SendSMS(ctx context.Context, phoneNumber, body string) error
}

type EmailSender interface {
	SendEmail(ctx context.Context, emailAddress, subject, body string) error
}

// DeviceLookup resolves a user's registered push tokens — Notifications
// doesn't own the devices table (Identity does, see 0003_auth.up.sql),
// so this is a small consumer-defined interface rather than a direct
// cross-module repository dependency.
type DeviceLookup interface {
	ListFCMTokensForUser(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// ContactLookup resolves a user's phone/email for SMS/email channels —
// same reasoning as DeviceLookup, implemented against the Users module.
type ContactLookup interface {
	GetContactInfo(ctx context.Context, userID uuid.UUID) (phone *string, email *string, err error)
}

// Repository is the persistence port for the Notifications module.
type Repository interface {
	Create(ctx context.Context, n *Notification) (*Notification, error)
	MarkSent(ctx context.Context, id uuid.UUID) error
	MarkFailed(ctx context.Context, id uuid.UUID, reason string) error
	MarkSkipped(ctx context.Context, id uuid.UUID) error

	ListForUser(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]*Notification, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int64, error)
	MarkRead(ctx context.Context, id, userID uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error

	GetPreference(ctx context.Context, userID uuid.UUID, category Category, channel Channel) (*Preference, error)
	ListPreferences(ctx context.Context, userID uuid.UUID) ([]*Preference, error)
	UpsertPreference(ctx context.Context, p *Preference) (*Preference, error)
}
