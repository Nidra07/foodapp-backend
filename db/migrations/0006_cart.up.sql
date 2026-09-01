-- Cart module (domain module 16 in the master spec).
--
-- One active cart per customer at a time, scoped to a single restaurant —
-- matches how Swiggy/Zomato force you to clear your cart when you start
-- ordering from a different restaurant. Cart contents reference LIVE menu
-- rows (menu_items, menu_item_variants, menu_addons) since a cart is
-- transient and should always reflect current prices/availability; the
-- moment of checkout is what snapshots everything into immutable
-- order_items (see 0007_orders.up.sql) — that boundary is the whole
-- reason Cart and Orders are separate modules.

CREATE TABLE carts (
  id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  customer_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  restaurant_id  UUID NOT NULL REFERENCES restaurants(id) ON DELETE CASCADE,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE (customer_id) -- enforces "one active cart per customer" at the DB level
);

CREATE TRIGGER trg_carts_updated_at
  BEFORE UPDATE ON carts
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE cart_items (
  id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  cart_id               UUID NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
  menu_item_id          UUID NOT NULL REFERENCES menu_items(id),
  variant_id            UUID REFERENCES menu_item_variants(id), -- null if item has no variant groups
  quantity              SMALLINT NOT NULL CHECK (quantity > 0),
  special_instructions  TEXT,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_cart_items_cart ON cart_items (cart_id);

CREATE TRIGGER trg_cart_items_updated_at
  BEFORE UPDATE ON cart_items
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Multi-select add-ons per cart item.
CREATE TABLE cart_item_addons (
  id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  cart_item_id  UUID NOT NULL REFERENCES cart_items(id) ON DELETE CASCADE,
  addon_id      UUID NOT NULL REFERENCES menu_addons(id),

  UNIQUE (cart_item_id, addon_id)
);

CREATE INDEX idx_cart_item_addons_cart_item ON cart_item_addons (cart_item_id);
