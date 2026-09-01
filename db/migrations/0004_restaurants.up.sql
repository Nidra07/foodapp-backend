-- Restaurants module: covers domain modules 5-10 from the master spec
-- (Restaurants, Restaurant Owners, Restaurant Staff, Restaurant
-- Documents/KYC, Restaurant Operating Hours, Restaurant Service Areas).
--
-- Ownership model: a restaurant has exactly one owning user
-- (owner_user_id -> users.id, role=restaurant_owner) plus zero or more
-- staff accounts with scoped permissions (restaurant_staff). This mirrors
-- how Zomato/Swiggy-style vendor accounts work: one legally-responsible
-- owner, delegated day-to-day staff.

CREATE TYPE restaurant_status AS ENUM (
  'draft',              -- onboarding started, not submitted
  'pending_approval',   -- submitted, awaiting admin review
  'approved',
  'rejected',
  'suspended',          -- admin action, e.g. repeated violations
  'closed'              -- permanently closed by owner or admin
);

CREATE TYPE kyc_status AS ENUM ('pending', 'under_review', 'verified', 'rejected');

CREATE TABLE restaurants (
  id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  owner_user_id       UUID NOT NULL REFERENCES users(id),
  name                VARCHAR(150) NOT NULL,
  slug                VARCHAR(180) NOT NULL UNIQUE,
  description         TEXT,
  cuisine_tags        TEXT[] NOT NULL DEFAULT '{}',
  status              restaurant_status NOT NULL DEFAULT 'draft',
  kyc_status          kyc_status NOT NULL DEFAULT 'pending',
  is_veg_only         BOOLEAN NOT NULL DEFAULT false,
  avg_prep_time_mins  SMALLINT NOT NULL DEFAULT 20,
  min_order_amount    NUMERIC(10,2) NOT NULL DEFAULT 0,
  commission_pct      NUMERIC(5,2) NOT NULL DEFAULT 20.00, -- platform commission; overridable per-restaurant by admin
  logo_url            TEXT,
  banner_url          TEXT,
  address_line1       VARCHAR(255) NOT NULL,
  address_line2       VARCHAR(255),
  city                VARCHAR(100) NOT NULL,
  state               VARCHAR(100) NOT NULL,
  postal_code         VARCHAR(20) NOT NULL,
  country             VARCHAR(2) NOT NULL DEFAULT 'IN',
  location            GEOGRAPHY(Point, 4326) NOT NULL, -- PostGIS point (lng, lat) for distance queries
  rating_avg          NUMERIC(3,2) NOT NULL DEFAULT 0,
  rating_count        INTEGER NOT NULL DEFAULT 0,
  is_accepting_orders BOOLEAN NOT NULL DEFAULT false,  -- owner-controlled "open now" toggle, independent of operating_hours
  approved_at         TIMESTAMPTZ,
  approved_by         UUID REFERENCES users(id),
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at          TIMESTAMPTZ
);

CREATE INDEX idx_restaurants_owner ON restaurants (owner_user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_restaurants_status ON restaurants (status) WHERE deleted_at IS NULL;
CREATE INDEX idx_restaurants_location ON restaurants USING GIST (location) WHERE deleted_at IS NULL;
CREATE INDEX idx_restaurants_city ON restaurants (city) WHERE deleted_at IS NULL;

CREATE TRIGGER trg_restaurants_updated_at
  BEFORE UPDATE ON restaurants
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TYPE staff_permission AS ENUM (
  'manage_menu',
  'manage_orders',
  'view_earnings',
  'manage_hours',
  'manage_staff'
);

CREATE TABLE restaurant_staff (
  id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  restaurant_id  UUID NOT NULL REFERENCES restaurants(id) ON DELETE CASCADE,
  user_id        UUID NOT NULL REFERENCES users(id),
  permissions    staff_permission[] NOT NULL DEFAULT '{}',
  invited_by     UUID NOT NULL REFERENCES users(id),
  status         VARCHAR(20) NOT NULL DEFAULT 'active', -- active | revoked
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE (restaurant_id, user_id)
);

CREATE INDEX idx_restaurant_staff_restaurant ON restaurant_staff (restaurant_id) WHERE status = 'active';
CREATE INDEX idx_restaurant_staff_user ON restaurant_staff (user_id) WHERE status = 'active';

CREATE TYPE document_type AS ENUM (
  'fssai_license',
  'gst_certificate',
  'pan_card',
  'business_registration',
  'bank_account_proof',
  'owner_identity_proof'
);

CREATE TABLE restaurant_documents (
  id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  restaurant_id  UUID NOT NULL REFERENCES restaurants(id) ON DELETE CASCADE,
  document_type  document_type NOT NULL,
  file_url       TEXT NOT NULL,            -- S3 object key/URL, uploaded via presigned URL (Files/Media module)
  document_number VARCHAR(100),            -- e.g. FSSAI license number, for admin cross-check
  status         kyc_status NOT NULL DEFAULT 'pending',
  rejection_reason TEXT,
  reviewed_by    UUID REFERENCES users(id),
  reviewed_at    TIMESTAMPTZ,
  expires_at     TIMESTAMPTZ,              -- licenses like FSSAI have renewal dates
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE (restaurant_id, document_type)
);

CREATE INDEX idx_restaurant_documents_restaurant ON restaurant_documents (restaurant_id);
CREATE INDEX idx_restaurant_documents_status ON restaurant_documents (status);

-- Operating hours: one row per (restaurant, day_of_week), supporting
-- split shifts (e.g. lunch 11-15, dinner 18-23) via multiple rows for
-- the same day.
CREATE TABLE restaurant_operating_hours (
  id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  restaurant_id  UUID NOT NULL REFERENCES restaurants(id) ON DELETE CASCADE,
  day_of_week    SMALLINT NOT NULL CHECK (day_of_week BETWEEN 0 AND 6), -- 0=Sunday
  open_time      TIME NOT NULL,
  close_time     TIME NOT NULL,
  is_closed      BOOLEAN NOT NULL DEFAULT false, -- explicit closed override for that day

  CONSTRAINT valid_hours CHECK (is_closed OR open_time < close_time)
);

CREATE INDEX idx_operating_hours_restaurant ON restaurant_operating_hours (restaurant_id, day_of_week);

-- Service areas: radius-based for Phase 2 simplicity (documented in
-- docs/assumptions.md). Polygon-based zones (matching the platform-wide
-- delivery_zones domain module) are a documented future upgrade.
CREATE TABLE restaurant_service_areas (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  restaurant_id   UUID NOT NULL REFERENCES restaurants(id) ON DELETE CASCADE,
  radius_km       NUMERIC(5,2) NOT NULL,
  is_active       BOOLEAN NOT NULL DEFAULT true,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE (restaurant_id)
);
