-- Notifications module (domain module ~26: Notifications).
--
-- This is a fan-out sink, not a message queue: `notifications` rows are
-- written synchronously by NotificationService.Notify and (for the
-- push/sms/email channels) sent synchronously too, best-effort, from
-- the calling request. There is no background worker/retry queue yet —
-- see docs/assumptions.md for why that's a real gap once volume grows,
-- and what the intended fix looks like (outbox table + worker, not
-- sending inline from request handlers).

CREATE TYPE notification_category AS ENUM (
  'order_placed',
  'order_confirmed',
  'order_preparing',
  'order_ready',
  'order_out_for_delivery',
  'order_delivered',
  'order_cancelled',
  'payment_captured',
  'payment_failed',
  'refund_processed',
  'delivery_assigned',
  'delivery_partner_arriving',
  'promotional',
  'account_security'
);

CREATE TYPE notification_channel AS ENUM ('push', 'sms', 'email', 'in_app');

CREATE TYPE notification_send_status AS ENUM ('pending', 'sent', 'failed', 'skipped');

CREATE TABLE notifications (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  category        notification_category NOT NULL,
  channel         notification_channel NOT NULL,
  title           VARCHAR(150) NOT NULL,
  body            TEXT NOT NULL,
  data            JSONB,                 -- deep-link payload, e.g. {"order_id": "..."} for the client to navigate on tap
  send_status     notification_send_status NOT NULL DEFAULT 'pending',
  failure_reason  TEXT,
  is_read         BOOLEAN NOT NULL DEFAULT false, -- only meaningful for channel = 'in_app'; other channels ignore this
  read_at         TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  sent_at         TIMESTAMPTZ
);

CREATE INDEX idx_notifications_user ON notifications (user_id, created_at DESC);
CREATE INDEX idx_notifications_user_unread ON notifications (user_id) WHERE channel = 'in_app' AND is_read = false;
CREATE INDEX idx_notifications_send_status ON notifications (send_status) WHERE send_status = 'pending';

-- Per-user, per-category, per-channel opt-out. Absence of a row means
-- "enabled" (default-on) — this table only ever stores explicit
-- disables, so a new category added later doesn't silently require a
-- data migration to keep existing users opted in.
CREATE TABLE notification_preferences (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  category    notification_category NOT NULL,
  channel     notification_channel NOT NULL,
  enabled     BOOLEAN NOT NULL, -- explicit true or false; a row only exists when the user has changed something away from the implicit default (enabled), see table comment
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE (user_id, category, channel)
);

CREATE INDEX idx_notification_preferences_user ON notification_preferences (user_id);
