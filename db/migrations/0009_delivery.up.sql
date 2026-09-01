-- Delivery module (domain modules ~23-25: Delivery Partners, Delivery
-- Assignment, Delivery Tracking).
--
-- Live location: delivery_partners.current_lat/lng is a periodic
-- snapshot updated by the partner app's location-ping endpoint, not a
-- full location-history table — see docs/assumptions.md for why
-- second-by-second tracking belongs in Redis (GEO + pub/sub for live
-- map updates), not Postgres, and why that isn't built yet.
--
-- Delivery confirmation OTP: reuses the "customer hands the delivery
-- partner a code" pattern already established by otp_purpose =
-- 'delivery_confirmation' in 0003_auth.up.sql, but stored directly on
-- the assignment row rather than through the Identity module's
-- otp_challenges table — that table is scoped to auth identifiers
-- (phone/email), whereas a delivery OTP is scoped to one specific
-- order/assignment and has different lifecycle rules (generated at
-- pickup, consumed at drop-off, never resent via SMS/email).

CREATE TYPE vehicle_type AS ENUM ('bike', 'scooter', 'bicycle', 'car', 'on_foot');

CREATE TABLE delivery_partners (
  id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id               UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
  vehicle_type          vehicle_type NOT NULL,
  vehicle_number        VARCHAR(20),
  license_number        VARCHAR(50),
  kyc_status            kyc_status NOT NULL DEFAULT 'pending', -- reuses the enum from 0004_restaurants.up.sql
  is_online             BOOLEAN NOT NULL DEFAULT false,        -- partner's own toggle: available for new assignments
  current_location      GEOGRAPHY(Point, 4326),
  last_location_update_at TIMESTAMPTZ,
  rating_avg            NUMERIC(3,2) NOT NULL DEFAULT 0,
  rating_count           INTEGER NOT NULL DEFAULT 0,
  active_assignment_count SMALLINT NOT NULL DEFAULT 0, -- denormalized, kept in sync by the application layer; used to cap concurrent deliveries
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_delivery_partners_online ON delivery_partners (is_online) WHERE kyc_status = 'verified';
CREATE INDEX idx_delivery_partners_location ON delivery_partners USING GIST (current_location) WHERE is_online = true;

CREATE TRIGGER trg_delivery_partners_updated_at
  BEFORE UPDATE ON delivery_partners
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TYPE assignment_status AS ENUM (
  'offered',    -- sent to a partner, awaiting accept/reject
  'accepted',
  'rejected',
  'picked_up',
  'delivered',
  'cancelled'   -- restaurant/admin/system cancelled the assignment (order cancelled, partner unreachable, etc.)
);

CREATE TABLE delivery_assignments (
  id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  order_id              UUID NOT NULL REFERENCES orders(id),
  restaurant_id         UUID NOT NULL REFERENCES restaurants(id), -- denormalized from orders, avoids a join for restaurant-side queue queries
  delivery_partner_id   UUID NOT NULL REFERENCES delivery_partners(id),
  status                assignment_status NOT NULL DEFAULT 'offered',
  delivery_otp          VARCHAR(6),          -- generated at pickup, shown to customer, entered by partner at drop-off
  offered_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  accepted_at           TIMESTAMPTZ,
  rejected_at           TIMESTAMPTZ,
  picked_up_at          TIMESTAMPTZ,
  delivered_at          TIMESTAMPTZ,
  cancelled_at          TIMESTAMPTZ,
  cancellation_reason   TEXT
);

CREATE INDEX idx_delivery_assignments_order ON delivery_assignments (order_id);
CREATE INDEX idx_delivery_assignments_partner ON delivery_assignments (delivery_partner_id, status);
CREATE INDEX idx_delivery_assignments_restaurant ON delivery_assignments (restaurant_id);

-- Only one NON-TERMINAL assignment per order at a time (an order can
-- accumulate multiple rejected/cancelled assignment rows over its
-- lifetime as it gets re-offered to different partners, but never two
-- simultaneously active ones).
CREATE UNIQUE INDEX idx_delivery_assignments_one_active_per_order
  ON delivery_assignments (order_id)
  WHERE status NOT IN ('rejected', 'cancelled');
