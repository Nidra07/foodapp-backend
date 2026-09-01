-- Orders module (domain module 17-19: Orders, Order Items, Order Status
-- Tracking). This is the checkout boundary: everything here is an
-- immutable SNAPSHOT of prices/names at the moment of order placement.
-- Order rows must never join back to live menu_items for pricing —
-- menu prices change, orders must not retroactively change with them.
-- (This is why order_items duplicates item_name/unit_price instead of
-- just storing menu_item_id + relying on a join.)

CREATE TYPE order_status AS ENUM (
  'placed',            -- customer completed checkout, awaiting restaurant confirmation
  'confirmed',         -- restaurant accepted the order
  'preparing',
  'ready_for_pickup',
  'out_for_delivery',
  'delivered',
  'cancelled'
);

CREATE TYPE payment_status AS ENUM ('pending', 'paid', 'failed', 'refunded');
CREATE TYPE payment_method AS ENUM ('cod', 'upi', 'card', 'wallet');

CREATE TABLE orders (
  id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  order_number          VARCHAR(20) NOT NULL UNIQUE, -- short human-readable code, e.g. "FD-8K3N2Q"
  customer_id           UUID NOT NULL REFERENCES users(id),
  restaurant_id         UUID NOT NULL REFERENCES restaurants(id),
  status                order_status NOT NULL DEFAULT 'placed',

  subtotal              NUMERIC(10,2) NOT NULL CHECK (subtotal >= 0),
  tax_amount            NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
  delivery_fee          NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (delivery_fee >= 0),
  discount_amount       NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
  total_amount          NUMERIC(10,2) NOT NULL CHECK (total_amount >= 0),

  payment_status        payment_status NOT NULL DEFAULT 'pending',
  payment_method        payment_method NOT NULL DEFAULT 'upi',

  -- Delivery address is snapshotted (not FK'd to a saved-addresses table,
  -- which doesn't exist yet as of Phase 3) so the order retains the
  -- correct address even if the customer edits/deletes it later.
  delivery_address_line1 VARCHAR(255) NOT NULL,
  delivery_address_line2 VARCHAR(255),
  delivery_city           VARCHAR(100) NOT NULL,
  delivery_state          VARCHAR(100) NOT NULL,
  delivery_postal_code    VARCHAR(20) NOT NULL,
  delivery_lat             DOUBLE PRECISION NOT NULL,
  delivery_lng             DOUBLE PRECISION NOT NULL,
  contact_phone            VARCHAR(20) NOT NULL,

  special_instructions   TEXT,
  cancellation_reason    TEXT,
  cancelled_by           UUID REFERENCES users(id),

  estimated_delivery_at  TIMESTAMPTZ,
  placed_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
  confirmed_at            TIMESTAMPTZ,
  ready_at                TIMESTAMPTZ,
  picked_up_at             TIMESTAMPTZ,
  delivered_at             TIMESTAMPTZ,
  cancelled_at             TIMESTAMPTZ,

  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_orders_customer ON orders (customer_id, created_at DESC);
CREATE INDEX idx_orders_restaurant ON orders (restaurant_id, created_at DESC);
CREATE INDEX idx_orders_status ON orders (status);
CREATE INDEX idx_orders_restaurant_status ON orders (restaurant_id, status) WHERE status NOT IN ('delivered', 'cancelled');

CREATE TRIGGER trg_orders_updated_at
  BEFORE UPDATE ON orders
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE order_items (
  id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  order_id         UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  menu_item_id     UUID REFERENCES menu_items(id) ON DELETE SET NULL, -- kept for analytics/reorder; not for pricing
  item_name        VARCHAR(150) NOT NULL,   -- snapshot
  variant_name     VARCHAR(100),            -- snapshot, null if no variant selected
  unit_price       NUMERIC(10,2) NOT NULL CHECK (unit_price >= 0), -- snapshot: base_price or variant price at order time
  quantity         SMALLINT NOT NULL CHECK (quantity > 0),
  line_total       NUMERIC(10,2) NOT NULL CHECK (line_total >= 0), -- (unit_price + sum(addon prices)) * quantity
  special_instructions TEXT
);

CREATE INDEX idx_order_items_order ON order_items (order_id);

CREATE TABLE order_item_addons (
  id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  order_item_id UUID NOT NULL REFERENCES order_items(id) ON DELETE CASCADE,
  addon_name    VARCHAR(100) NOT NULL,  -- snapshot
  addon_price   NUMERIC(10,2) NOT NULL CHECK (addon_price >= 0) -- snapshot
);

CREATE INDEX idx_order_item_addons_order_item ON order_item_addons (order_item_id);

-- Full audit trail of every status transition, independent of the
-- timestamp columns on `orders` (which only hold the latest occurrence
-- of each milestone). Needed for support/dispute resolution ("who
-- cancelled this and when") and for the Admin panel's order timeline UI.
CREATE TABLE order_status_history (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  order_id    UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  status      order_status NOT NULL,
  changed_by  UUID REFERENCES users(id), -- null = system-triggered (e.g. auto-cancel on timeout)
  notes       TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_order_status_history_order ON order_status_history (order_id, created_at);
