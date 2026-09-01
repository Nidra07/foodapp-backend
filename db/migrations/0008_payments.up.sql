-- Payments module (domain modules ~20-22: Payment Transactions, Saved
-- Payment Methods, Refunds).
--
-- Gateway-agnostic by design: every gateway-specific field is a plain
-- string/JSONB column (gateway name, gateway's own transaction/order
-- IDs, raw webhook payload) rather than typed columns per provider. The
-- actual Razorpay/Stripe/etc. integration lives entirely behind the
-- domain.PaymentGateway interface in the Go code — this schema doesn't
-- change when the gateway does. See docs/assumptions.md.

CREATE TYPE payment_transaction_status AS ENUM (
  'initiated',   -- created on our side, customer redirected to gateway checkout
  'authorized',  -- gateway confirmed funds are reserved (e.g. card auth) but not yet captured
  'captured',    -- funds captured — this is "paid" from the platform's perspective
  'failed',
  'refunded',
  'partially_refunded'
);

CREATE TABLE payment_transactions (
  id                     UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  order_id               UUID NOT NULL REFERENCES orders(id),
  customer_id            UUID NOT NULL REFERENCES users(id),
  amount                 NUMERIC(10,2) NOT NULL CHECK (amount >= 0),
  currency               VARCHAR(3) NOT NULL DEFAULT 'INR',
  method                 payment_method NOT NULL, -- reuses the enum from 0007_orders.up.sql (upi/card/wallet/cod)
  status                 payment_transaction_status NOT NULL DEFAULT 'initiated',
  gateway                VARCHAR(30) NOT NULL,           -- e.g. "razorpay", "mock"
  gateway_order_id       VARCHAR(150),                   -- gateway's order/intent id, created at initiation
  gateway_payment_id     VARCHAR(150),                   -- gateway's payment id, set once the customer pays
  gateway_signature      TEXT,                           -- for verifying the client-side callback (HMAC etc.)
  failure_reason         TEXT,
  raw_webhook_payload    JSONB,                          -- last webhook received for this transaction, for support/debugging
  initiated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at           TIMESTAMPTZ,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payment_transactions_order ON payment_transactions (order_id);
CREATE INDEX idx_payment_transactions_customer ON payment_transactions (customer_id);
CREATE INDEX idx_payment_transactions_gateway_order ON payment_transactions (gateway, gateway_order_id);
CREATE INDEX idx_payment_transactions_status ON payment_transactions (status);

CREATE TRIGGER trg_payment_transactions_updated_at
  BEFORE UPDATE ON payment_transactions
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Saved payment methods (tokenized — we never store raw card/UPI
-- numbers, only the gateway's reusable token, per PCI-DSS scope
-- reduction). Optional convenience feature; COD/one-off UPI don't need
-- a row here.
CREATE TABLE saved_payment_methods (
  id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  customer_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  method            payment_method NOT NULL,
  gateway           VARCHAR(30) NOT NULL,
  gateway_token     VARCHAR(255) NOT NULL,  -- opaque token from the gateway, never a raw PAN/VPA
  display_label     VARCHAR(100) NOT NULL,  -- e.g. "UPI - user@okhdfcbank", "Visa •••• 4242" — safe to show, contains no secret
  is_default        BOOLEAN NOT NULL DEFAULT false,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE (customer_id, gateway, gateway_token)
);

CREATE INDEX idx_saved_payment_methods_customer ON saved_payment_methods (customer_id);

CREATE TYPE refund_status AS ENUM ('pending', 'processing', 'completed', 'failed');

CREATE TABLE refunds (
  id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  payment_transaction_id  UUID NOT NULL REFERENCES payment_transactions(id),
  order_id                UUID NOT NULL REFERENCES orders(id),
  amount                  NUMERIC(10,2) NOT NULL CHECK (amount > 0),
  reason                  TEXT NOT NULL,
  status                  refund_status NOT NULL DEFAULT 'pending',
  gateway_refund_id       VARCHAR(150),
  initiated_by            UUID REFERENCES users(id), -- null = system-initiated (e.g. auto-refund on cancellation)
  failure_reason          TEXT,
  initiated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at            TIMESTAMPTZ
);

CREATE INDEX idx_refunds_transaction ON refunds (payment_transaction_id);
CREATE INDEX idx_refunds_order ON refunds (order_id);
