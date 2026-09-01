// Package application orchestrates Notifications use cases. Notify is
// the single entrypoint every other module calls to notify a user of an
// event — it fans out across channels, checks preferences, and writes a
// durable row per (category, channel) attempt before sending, so a send
// failure never means "we don't know this was supposed to happen."
package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/foodapp/backend/internal/modules/notifications/domain"
	"github.com/foodapp/backend/internal/platform/logger"
)

// defaultChannels defines which channels each category uses out of the
// box (before per-user preference overrides are applied). Order/payment/
// delivery events go to push + in-app (immediate, actionable); account
// security events add SMS (higher assurance channel for things like
// "your password changed"); promotional is push + in_app only, never
// SMS/email by default, since unsolicited SMS/email is the most likely
// channel to generate spam complaints — this is a product policy
// decision baked into code, flagged here so it's visible and easy to
// revisit rather than buried.
var defaultChannels = map[domain.Category][]domain.Channel{
	domain.CategoryOrderPlaced:             {domain.ChannelPush, domain.ChannelInApp},
	domain.CategoryOrderConfirmed:          {domain.ChannelPush, domain.ChannelInApp},
	domain.CategoryOrderPreparing:          {domain.ChannelInApp},
	domain.CategoryOrderReady:              {domain.ChannelPush, domain.ChannelInApp},
	domain.CategoryOrderOutForDelivery:     {domain.ChannelPush, domain.ChannelInApp},
	domain.CategoryOrderDelivered:          {domain.ChannelPush, domain.ChannelInApp},
	domain.CategoryOrderCancelled:          {domain.ChannelPush, domain.ChannelInApp, domain.ChannelSMS},
	domain.CategoryPaymentCaptured:         {domain.ChannelInApp},
	domain.CategoryPaymentFailed:           {domain.ChannelPush, domain.ChannelInApp},
	domain.CategoryRefundProcessed:         {domain.ChannelPush, domain.ChannelInApp, domain.ChannelEmail},
	domain.CategoryDeliveryAssigned:        {domain.ChannelPush, domain.ChannelInApp},
	domain.CategoryDeliveryPartnerArriving: {domain.ChannelPush},
	domain.CategoryPromotional:             {domain.ChannelPush, domain.ChannelInApp},
	domain.CategoryAccountSecurity:         {domain.ChannelPush, domain.ChannelInApp, domain.ChannelSMS},
}

type NotificationService struct {
	repo    domain.Repository
	push    domain.PushSender
	sms     domain.SMSSender
	email   domain.EmailSender
	devices domain.DeviceLookup
	contact domain.ContactLookup
	log     *logger.Logger
}

func NewNotificationService(
	repo domain.Repository, push domain.PushSender, sms domain.SMSSender, email domain.EmailSender,
	devices domain.DeviceLookup, contact domain.ContactLookup, log *logger.Logger,
) *NotificationService {
	return &NotificationService{repo: repo, push: push, sms: sms, email: email, devices: devices, contact: contact, log: log}
}

// Notify is deliberately synchronous and best-effort: it never returns
// an error to the caller for a send failure (only for a database write
// failure on the notification record itself), because a failed SMS
// should not, for example, roll back an order status transition that
// already genuinely happened. See docs/assumptions.md for why this
// being synchronous-from-the-request-path (rather than an outbox +
// background worker) is a real scalability gap, not a design choice
// meant to last.
func (s *NotificationService) Notify(ctx context.Context, in domain.NotifyInput) {
	channels := defaultChannels[in.Category]
	if len(channels) == 0 {
		channels = []domain.Channel{domain.ChannelInApp}
	}

	for _, channel := range channels {
		if enabled, err := s.isEnabled(ctx, in.UserID, in.Category, channel); err != nil || !enabled {
			continue
		}

		record, err := s.repo.Create(ctx, &domain.Notification{
			UserID: in.UserID, Category: in.Category, Channel: channel, Title: in.Title, Body: in.Body, Data: in.Data,
		})
		if err != nil {
			s.log.Error().Err(err).Str("category", string(in.Category)).Str("channel", string(channel)).Msg("failed to persist notification")
			continue
		}

		s.send(ctx, record)
	}
}

func (s *NotificationService) send(ctx context.Context, n *domain.Notification) {
	var err error
	switch n.Channel {
	case domain.ChannelInApp:
		// In-app notifications are "sent" the moment they're persisted —
		// the client polls/fetches the feed, there's no separate delivery step.
		_ = s.repo.MarkSent(ctx, n.ID)
		return
	case domain.ChannelPush:
		tokens, lookupErr := s.devices.ListFCMTokensForUser(ctx, n.UserID)
		if lookupErr != nil || len(tokens) == 0 {
			_ = s.repo.MarkSkipped(ctx, n.ID)
			return
		}
		for _, token := range tokens {
			if sendErr := s.push.SendPush(ctx, token, n.Title, n.Body, n.Data); sendErr != nil {
				err = sendErr // best-effort across multiple devices: keep going, report the last error if any
			}
		}
	case domain.ChannelSMS:
		phone, _, lookupErr := s.contact.GetContactInfo(ctx, n.UserID)
		if lookupErr != nil || phone == nil {
			_ = s.repo.MarkSkipped(ctx, n.ID)
			return
		}
		err = s.sms.SendSMS(ctx, *phone, n.Body)
	case domain.ChannelEmail:
		_, email, lookupErr := s.contact.GetContactInfo(ctx, n.UserID)
		if lookupErr != nil || email == nil {
			_ = s.repo.MarkSkipped(ctx, n.ID)
			return
		}
		err = s.email.SendEmail(ctx, *email, n.Title, n.Body)
	}

	if err != nil {
		_ = s.repo.MarkFailed(ctx, n.ID, err.Error())
		s.log.Warn().Err(err).Str("channel", string(n.Channel)).Str("user_id", n.UserID.String()).Msg("notification send failed")
		return
	}
	_ = s.repo.MarkSent(ctx, n.ID)
}

func (s *NotificationService) isEnabled(ctx context.Context, userID uuid.UUID, category domain.Category, channel domain.Channel) (bool, error) {
	pref, err := s.repo.GetPreference(ctx, userID, category, channel)
	if err != nil {
		return true, nil // no explicit preference row = enabled by default, see 0010_notifications.up.sql table comment
	}
	return pref.Enabled, nil
}

func (s *NotificationService) ListForUser(ctx context.Context, userID uuid.UUID, page, pageSize int) ([]*domain.Notification, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}
	return s.repo.ListForUser(ctx, userID, page, pageSize)
}

func (s *NotificationService) CountUnread(ctx context.Context, userID uuid.UUID) (int64, error) {
	return s.repo.CountUnread(ctx, userID)
}

func (s *NotificationService) MarkRead(ctx context.Context, id, userID uuid.UUID) error {
	return s.repo.MarkRead(ctx, id, userID)
}

func (s *NotificationService) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	return s.repo.MarkAllRead(ctx, userID)
}

func (s *NotificationService) ListPreferences(ctx context.Context, userID uuid.UUID) ([]*domain.Preference, error) {
	return s.repo.ListPreferences(ctx, userID)
}

func (s *NotificationService) SetPreference(ctx context.Context, userID uuid.UUID, category domain.Category, channel domain.Channel, enabled bool) (*domain.Preference, error) {
	return s.repo.UpsertPreference(ctx, &domain.Preference{UserID: userID, Category: category, Channel: channel, Enabled: enabled})
}
