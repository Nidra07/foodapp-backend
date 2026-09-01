-- Admin RBAC module (domain module ~28: fine-grained admin permissions).
--
-- Every admin route built in Phases 2-7 was gated by the coarse
-- `RequireRole("admin")` check — anyone with users.primary_role='admin'
-- could approve restaurants, issue refunds, pay out settlements, review
-- delivery partner KYC, all of it. This module adds a finer-grained
-- layer ON TOP of that (a permission catalog + per-admin-user role
-- assignment + an audit trail), but does NOT retroactively replace the
-- coarse checks on every existing admin route — see docs/assumptions.md
-- for exactly which routes were and weren't retrofitted, and why a full
-- retrofit was out of scope for this phase.

CREATE TABLE admin_roles (
  id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  name         VARCHAR(50) NOT NULL UNIQUE, -- e.g. "super_admin", "finance_admin", "support_agent", "content_moderator"
  description  TEXT,
  is_system    BOOLEAN NOT NULL DEFAULT false, -- system roles (e.g. super_admin) can't be deleted via the API
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Fixed permission catalog, seeded below. Permission codes follow
-- "resource.action" (e.g. "restaurants.approve", "settlements.pay") so
-- they read naturally in both code and an eventual admin-panel UI, and
-- so new permissions for future modules can follow the same convention
-- without a schema change.
CREATE TABLE admin_permissions (
  id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  code         VARCHAR(100) NOT NULL UNIQUE,
  description  TEXT NOT NULL
);

CREATE TABLE admin_role_permissions (
  role_id        UUID NOT NULL REFERENCES admin_roles(id) ON DELETE CASCADE,
  permission_id  UUID NOT NULL REFERENCES admin_permissions(id) ON DELETE CASCADE,
  PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE admin_user_roles (
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role_id     UUID NOT NULL REFERENCES admin_roles(id) ON DELETE CASCADE,
  granted_by  UUID REFERENCES users(id),
  granted_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_admin_user_roles_user ON admin_user_roles (user_id);

-- Audit log of admin actions. Deliberately append-only (no UPDATE/DELETE
-- queries are ever written against this table) since an audit trail
-- that can be edited after the fact isn't an audit trail. Not every
-- admin action across the whole platform writes here yet — see
-- docs/assumptions.md for which ones were retrofitted in this phase.
CREATE TABLE admin_audit_log (
  id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  admin_user_id  UUID NOT NULL REFERENCES users(id),
  action         VARCHAR(100) NOT NULL,        -- e.g. "restaurant.approve", "settlement.pay", "refund.issue"
  resource_type  VARCHAR(50) NOT NULL,         -- e.g. "restaurant", "restaurant_settlement", "payment_transaction"
  resource_id    UUID,
  details        JSONB,                        -- action-specific context (e.g. {"reason": "...", "amount": 360})
  ip_address     INET,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_admin_audit_log_admin_user ON admin_audit_log (admin_user_id, created_at DESC);
CREATE INDEX idx_admin_audit_log_resource ON admin_audit_log (resource_type, resource_id);
CREATE INDEX idx_admin_audit_log_created ON admin_audit_log (created_at DESC);

-- Seed the permission catalog and a super_admin role with every
-- permission — every OTHER role (finance_admin, support_agent, etc.) is
-- expected to be created by an operator via the API with a deliberately
-- chosen subset, not seeded here, since that subset is a business
-- decision this codebase shouldn't guess at.
INSERT INTO admin_permissions (code, description) VALUES
  ('restaurants.approve', 'Approve or reject restaurant onboarding submissions'),
  ('restaurants.suspend', 'Suspend or close a restaurant'),
  ('restaurants.documents.review', 'Review restaurant KYC documents'),
  ('delivery.partners.approve', 'Approve or reject delivery partner KYC'),
  ('orders.view_all', 'View any order regardless of ownership'),
  ('payments.refund', 'Issue a payment refund'),
  ('settlements.manage_cycles', 'Open and process settlement cycles'),
  ('settlements.pay', 'Mark a settlement as paid'),
  ('users.suspend', 'Suspend or reactivate a user account'),
  ('admin.manage_roles', 'Create/edit admin roles and permission assignments'),
  ('admin.assign_roles', 'Grant or revoke admin roles for a user');

INSERT INTO admin_roles (name, description, is_system)
VALUES ('super_admin', 'Full platform access — every permission in the catalog', true);

INSERT INTO admin_role_permissions (role_id, permission_id)
SELECT (SELECT id FROM admin_roles WHERE name = 'super_admin'), id FROM admin_permissions;
