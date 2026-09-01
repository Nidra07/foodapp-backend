-- Core "users" table: one row per human identity in the system,
-- regardless of role. Role-specific profile data (customer preferences,
-- restaurant ownership, delivery partner vehicle info) lives in separate
-- tables (customers, restaurant_owners, delivery_partners) that FK back
-- here — see docs/model-catalog.md. This keeps auth/identity decoupled
-- from role-specific business data.

CREATE TYPE user_role AS ENUM (
  'customer',
  'restaurant_owner',
  'restaurant_staff',
  'delivery_partner',
  'admin'
);

CREATE TYPE user_status AS ENUM (
  'active',
  'suspended',
  'deactivated',
  'pending_verification'
);

CREATE TABLE users (
  id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  phone_number       VARCHAR(20)  UNIQUE,          -- E.164 format, e.g. +919876543210
  email              CITEXT UNIQUE,
  full_name          VARCHAR(150),
  primary_role       user_role NOT NULL,
  status             user_status NOT NULL DEFAULT 'pending_verification',
  phone_verified_at  TIMESTAMPTZ,
  email_verified_at  TIMESTAMPTZ,
  profile_image_url  TEXT,
  last_login_at      TIMESTAMPTZ,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at         TIMESTAMPTZ,                  -- soft delete: never hard-delete user rows (audit/FK integrity)

  CONSTRAINT users_phone_or_email_required CHECK (phone_number IS NOT NULL OR email IS NOT NULL)
);

CREATE INDEX idx_users_phone_number ON users (phone_number) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_email ON users (email) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_role_status ON users (primary_role, status) WHERE deleted_at IS NULL;

-- Users can hold more than one role over time (e.g. a customer who also
-- becomes a delivery partner). primary_role drives default app routing;
-- this table tracks every role ever granted for authorization checks.
CREATE TABLE user_roles (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role        user_role NOT NULL,
  granted_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at  TIMESTAMPTZ,

  UNIQUE (user_id, role)
);

CREATE INDEX idx_user_roles_user_id ON user_roles (user_id) WHERE revoked_at IS NULL;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_updated_at
  BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
