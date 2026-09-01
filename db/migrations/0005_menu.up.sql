-- Menu module: covers domain modules 11-15 from the master spec
-- (Food/Menu Categories, Menu Items, Menu Variants, Menu Add-ons, Menu
-- Availability).
--
-- Pricing model: menu_items.base_price is the price for the item with no
-- variant selected. If variants exist, the client is expected to require
-- a variant selection (enforced at cart-time in the Cart module, not
-- here) and menu_item_variants.price REPLACES base_price, it does not
-- add to it. Add-ons always ADD to whichever price was selected. This
-- mirrors how Swiggy/Zomato structure variants ("Half/Full", "Small/
-- Medium/Large") vs add-ons ("Extra cheese", "Extra raita").

CREATE TABLE menu_categories (
  id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  restaurant_id  UUID NOT NULL REFERENCES restaurants(id) ON DELETE CASCADE,
  name           VARCHAR(100) NOT NULL,
  description    TEXT,
  display_order  INTEGER NOT NULL DEFAULT 0,
  is_active      BOOLEAN NOT NULL DEFAULT true,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE (restaurant_id, name)
);

CREATE INDEX idx_menu_categories_restaurant ON menu_categories (restaurant_id, display_order);

CREATE TRIGGER trg_menu_categories_updated_at
  BEFORE UPDATE ON menu_categories
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TYPE food_type AS ENUM ('veg', 'non_veg', 'egg');

CREATE TABLE menu_items (
  id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  restaurant_id    UUID NOT NULL REFERENCES restaurants(id) ON DELETE CASCADE,
  category_id      UUID NOT NULL REFERENCES menu_categories(id) ON DELETE CASCADE,
  name             VARCHAR(150) NOT NULL,
  description      TEXT,
  food_type        food_type NOT NULL DEFAULT 'veg',
  base_price       NUMERIC(10,2) NOT NULL CHECK (base_price >= 0),
  image_url        TEXT,
  is_available     BOOLEAN NOT NULL DEFAULT true,  -- owner/staff toggle: "out of stock today"
  is_active        BOOLEAN NOT NULL DEFAULT true,   -- soft-disable: hidden from customers, distinct from is_available
  display_order    INTEGER NOT NULL DEFAULT 0,
  calories         INTEGER,
  is_bestseller    BOOLEAN NOT NULL DEFAULT false,
  spice_level      SMALLINT CHECK (spice_level BETWEEN 0 AND 3),
  prep_time_mins   SMALLINT,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at       TIMESTAMPTZ
);

CREATE INDEX idx_menu_items_restaurant ON menu_items (restaurant_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_menu_items_category ON menu_items (category_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_menu_items_available ON menu_items (restaurant_id, is_available) WHERE deleted_at IS NULL AND is_active = true;

CREATE TRIGGER trg_menu_items_updated_at
  BEFORE UPDATE ON menu_items
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Variant GROUPS allow modeling "Size" (Half/Full) as one selectable
-- group per item; is_required + max_select=1 gives single-select
-- (the common case). Multi-select variant groups (rare, but some
-- platforms allow it) are supported via max_select > 1 without a schema
-- change.
CREATE TABLE menu_item_variant_groups (
  id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  menu_item_id   UUID NOT NULL REFERENCES menu_items(id) ON DELETE CASCADE,
  name           VARCHAR(100) NOT NULL, -- e.g. "Size", "Spice Level"
  is_required    BOOLEAN NOT NULL DEFAULT true,
  min_select     SMALLINT NOT NULL DEFAULT 1,
  max_select     SMALLINT NOT NULL DEFAULT 1,
  display_order  INTEGER NOT NULL DEFAULT 0,

  CONSTRAINT valid_select_range CHECK (min_select <= max_select)
);

CREATE INDEX idx_variant_groups_item ON menu_item_variant_groups (menu_item_id);

CREATE TABLE menu_item_variants (
  id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  variant_group_id  UUID NOT NULL REFERENCES menu_item_variant_groups(id) ON DELETE CASCADE,
  name              VARCHAR(100) NOT NULL, -- e.g. "Half", "Full"
  price             NUMERIC(10,2) NOT NULL CHECK (price >= 0), -- replaces item base_price when selected
  is_available      BOOLEAN NOT NULL DEFAULT true,
  display_order     INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_variants_group ON menu_item_variants (variant_group_id);

-- Add-on GROUPS (e.g. "Toppings", "Extra sides") attach to an item and
-- contain individual add-ons that ADD to the selected price.
CREATE TABLE menu_addon_groups (
  id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  menu_item_id   UUID NOT NULL REFERENCES menu_items(id) ON DELETE CASCADE,
  name           VARCHAR(100) NOT NULL,
  is_required    BOOLEAN NOT NULL DEFAULT false,
  min_select     SMALLINT NOT NULL DEFAULT 0,
  max_select     SMALLINT NOT NULL DEFAULT 1,
  display_order  INTEGER NOT NULL DEFAULT 0,

  CONSTRAINT valid_addon_select_range CHECK (min_select <= max_select)
);

CREATE INDEX idx_addon_groups_item ON menu_addon_groups (menu_item_id);

CREATE TABLE menu_addons (
  id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  addon_group_id UUID NOT NULL REFERENCES menu_addon_groups(id) ON DELETE CASCADE,
  name           VARCHAR(100) NOT NULL,
  price          NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (price >= 0),
  is_available   BOOLEAN NOT NULL DEFAULT true,
  display_order  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_addons_group ON menu_addons (addon_group_id);
