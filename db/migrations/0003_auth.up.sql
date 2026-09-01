-- Identity & Authentication module: OTP challenges, refresh tokens,
-- device/session tracking. Access tokens are stateless JWTs (not stored);
-- refresh tokens ARE stored (hashed) so they can be revoked individually
-- or in bulk (logout-all-devices, security incident).

CREATE TYPE otp_purpose AS ENUM (
  'login',
  'signup',
  'phone_verification',
  'email_verification',
  'password_reset',
  'delivery_confirmation'
);

CREATE TABLE otp_challenges (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  identifier      VARCHAR(150) NOT NULL,        -- phone or email the OTP was sent to
  purpose         otp_purpose NOT NULL,
  code_hash       TEXT NOT NULL,                -- never store OTP in plaintext
  attempt_count   SMALLINT NOT NULL DEFAULT 0,
  max_attempts    SMALLINT NOT NULL DEFAULT 5,
  expires_at      TIMESTAMPTZ NOT NULL,
  consumed_at     TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  ip_address      INET,
  user_agent      TEXT
);

CREATE INDEX idx_otp_identifier_purpose ON otp_challenges (identifier, purpose, created_at DESC);
-- Fast lookup of the latest unconsumed OTP for verification / resend-cooldown checks.
CREATE INDEX idx_otp_active ON otp_challenges (identifier, purpose) WHERE consumed_at IS NULL;

CREATE TABLE devices (
  id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  device_id     VARCHAR(255) NOT NULL,          -- client-generated stable device identifier
  platform      VARCHAR(20) NOT NULL,           -- ios | android | web
  fcm_token     TEXT,
  app_version   VARCHAR(20),
  last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE (user_id, device_id)
);

CREATE INDEX idx_devices_user_id ON devices (user_id);

CREATE TABLE refresh_tokens (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  device_id       UUID REFERENCES devices(id) ON DELETE SET NULL,
  token_hash      TEXT NOT NULL UNIQUE,          -- SHA-256 of the raw refresh token
  family_id       UUID NOT NULL,                 -- rotation family: reused-token detection revokes the whole family
  issued_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at      TIMESTAMPTZ NOT NULL,
  revoked_at      TIMESTAMPTZ,
  replaced_by_id  UUID REFERENCES refresh_tokens(id),
  ip_address      INET,
  user_agent      TEXT
);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens (user_id) WHERE revoked_at IS NULL;
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens (family_id);

CREATE TABLE login_attempts (
  id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  identifier    VARCHAR(150) NOT NULL,
  success       BOOLEAN NOT NULL,
  ip_address    INET,
  user_agent    TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_login_attempts_identifier ON login_attempts (identifier, created_at DESC);
