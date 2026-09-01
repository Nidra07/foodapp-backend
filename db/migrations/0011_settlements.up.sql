-- Settlements module (domain module ~27: Restaurant/Partner Settlements).
--
-- A settlement CYCLE is a fixed date range (e.g. one week) that gets
-- "closed out": every restaurant that had delivered+paid orders in that
-- range gets one restaurant_settlements row summarizing what the
-- platform owes them (order subtotal minus commission); every delivery
-- partner that completed deliveries in that range gets one
-- delivery_settlements row (flat per-delivery fee, no commission —
-- partners aren't charged commission on their own earnings). This
-- mirrors how gig-economy platforms typically batch payouts rather than
-- paying out after every single order, which would be far more gateway
-- transaction fees for negligible benefit.
--
-- Payout accounts store only a masked account number (last 4 digits) —
-- same PCI/data-minimization posture as saved_payment_methods in
-- 0008_payments.up.sql. A real implementation would tokenize the full
-- account/routing details with a payout provider (Razorpay X, Stripe
-- Connect, etc.) rather than storing them at all — see docs/assumptions.md.

CREATE TYPE payout_owner_type AS ENUM ('restaurant', 'delivery_partner');

CREATE TABLE payout_accounts (
  id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  owner_type            payout_owner_type NOT NULL,
  owner_id              UUID NOT NULL, -- restaurants.id or delivery_partners.id depending on owner_type; no single FK target possible, enforced in application code
  account_holder_name   VARCHAR(150) NOT NULL,
  account_number_last4  VARCHAR(4) NOT NULL,
  account_token         VARCHAR(255) NOT NULL, -- opaque payout-provider token for the full account details; never store the full account number in this table
  ifsc_code             VARCHAR(20) NOT NULL,
  bank_name             VARCHAR(100),
  is_verified           BOOLEAN NOT NULL DEFAULT false,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE (owner_type, owner_id)
);

CREATE INDEX idx_payout_accounts_owner ON payout_accounts (owner_type, owner_id);

CREATE TYPE cycle_status AS ENUM ('open', 'processing', 'completed');

CREATE TABLE settlement_cycles (
  id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  cycle_start   DATE NOT NULL,
  cycle_end     DATE NOT NULL, -- exclusive: orders/deliveries counted are >= cycle_start AND < cycle_end
  status        cycle_status NOT NULL DEFAULT 'open',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  processed_at  TIMESTAMPTZ,

  CONSTRAINT valid_cycle_range CHECK (cycle_start < cycle_end)
);

CREATE UNIQUE INDEX idx_settlement_cycles_range ON settlement_cycles (cycle_start, cycle_end);

CREATE TYPE settlement_status AS ENUM ('pending', 'processing', 'paid', 'failed');

CREATE TABLE restaurant_settlements (
  id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  cycle_id          UUID NOT NULL REFERENCES settlement_cycles(id),
  restaurant_id     UUID NOT NULL REFERENCES restaurants(id),
  order_count       INTEGER NOT NULL DEFAULT 0,
  gross_subtotal    NUMERIC(12,2) NOT NULL DEFAULT 0, -- sum of order.subtotal for delivered+paid orders in the cycle (excludes tax/delivery fee, which pass through to the platform/government/logistics, not the restaurant)
  commission_amount NUMERIC(12,2) NOT NULL DEFAULT 0, -- gross_subtotal * restaurant's commission_pct at settlement time
  net_payable       NUMERIC(12,2) NOT NULL DEFAULT 0, -- gross_subtotal - commission_amount
  status            settlement_status NOT NULL DEFAULT 'pending',
  payout_account_id UUID REFERENCES payout_accounts(id),
  payout_reference  VARCHAR(150),
  failure_reason    TEXT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  paid_at           TIMESTAMPTZ,

  UNIQUE (cycle_id, restaurant_id)
);

CREATE INDEX idx_restaurant_settlements_restaurant ON restaurant_settlements (restaurant_id);
CREATE INDEX idx_restaurant_settlements_cycle ON restaurant_settlements (cycle_id);
CREATE INDEX idx_restaurant_settlements_status ON restaurant_settlements (status) WHERE status = 'pending';

CREATE TABLE delivery_settlements (
  id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  cycle_id            UUID NOT NULL REFERENCES settlement_cycles(id),
  delivery_partner_id UUID NOT NULL REFERENCES delivery_partners(id),
  delivery_count      INTEGER NOT NULL DEFAULT 0,
  gross_earnings      NUMERIC(12,2) NOT NULL DEFAULT 0, -- delivery_count * flat per-delivery fee at settlement time
  incentive_amount    NUMERIC(12,2) NOT NULL DEFAULT 0, -- placeholder for future bonus/incentive schemes; always 0 for now, see docs/assumptions.md
  net_payable         NUMERIC(12,2) NOT NULL DEFAULT 0, -- gross_earnings + incentive_amount
  status              settlement_status NOT NULL DEFAULT 'pending',
  payout_account_id   UUID REFERENCES payout_accounts(id),
  payout_reference    VARCHAR(150),
  failure_reason      TEXT,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  paid_at             TIMESTAMPTZ,

  UNIQUE (cycle_id, delivery_partner_id)
);

CREATE INDEX idx_delivery_settlements_partner ON delivery_settlements (delivery_partner_id);
CREATE INDEX idx_delivery_settlements_cycle ON delivery_settlements (cycle_id);
CREATE INDEX idx_delivery_settlements_status ON delivery_settlements (status) WHERE status = 'pending';
