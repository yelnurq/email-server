-- +goose Up
-- Identity & RBAC baseline: tenants, organizations, users, credentials,
-- sessions, roles/permissions, audit log.

CREATE TABLE tenants (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    slug        citext NOT NULL UNIQUE,
    status      text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE organizations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        text NOT NULL,
    slug        citext NOT NULL,
    status      text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug)
);
CREATE INDEX organizations_tenant_idx ON organizations (tenant_id);

CREATE TABLE users (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    organization_id uuid REFERENCES organizations(id) ON DELETE SET NULL,
    email           citext NOT NULL UNIQUE,
    display_name    text NOT NULL DEFAULT '',
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX users_tenant_idx ON users (tenant_id);
CREATE INDEX users_organization_idx ON users (organization_id);

CREATE TABLE user_credentials (
    user_id        uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    password_hash  text NOT NULL, -- PHC string, e.g. $argon2id$v=19$m=65536,t=3,p=4$...
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  bytea NOT NULL UNIQUE, -- sha256 of the opaque bearer token
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    ip          text,
    user_agent  text
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

CREATE TABLE roles (
    code        text PRIMARY KEY,
    name        text NOT NULL,
    description text NOT NULL DEFAULT ''
);

CREATE TABLE permissions (
    code        text PRIMARY KEY,
    description text NOT NULL DEFAULT ''
);

CREATE TABLE role_permissions (
    role_code       text NOT NULL REFERENCES roles(code) ON DELETE CASCADE,
    permission_code text NOT NULL REFERENCES permissions(code) ON DELETE CASCADE,
    PRIMARY KEY (role_code, permission_code)
);

CREATE TABLE user_roles (
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_code  text NOT NULL REFERENCES roles(code) ON DELETE CASCADE,
    -- scope of the role grant: whole platform, one tenant, or one organization
    scope_type text NOT NULL CHECK (scope_type IN ('platform', 'tenant', 'organization')),
    scope_id   uuid, -- NULL for platform scope
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((scope_type = 'platform') = (scope_id IS NULL))
);
CREATE UNIQUE INDEX user_roles_unique_idx
    ON user_roles (user_id, role_code, scope_type, COALESCE(scope_id, '00000000-0000-0000-0000-000000000000'::uuid));
CREATE INDEX user_roles_user_idx ON user_roles (user_id);

CREATE TABLE audit_logs (
    id            bigserial PRIMARY KEY,
    tenant_id     uuid REFERENCES tenants(id) ON DELETE SET NULL,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action        text NOT NULL,
    resource_type text NOT NULL DEFAULT '',
    resource_id   text NOT NULL DEFAULT '',
    detail        jsonb NOT NULL DEFAULT '{}',
    ip            text,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_tenant_idx ON audit_logs (tenant_id, created_at DESC);

-- Reference data ------------------------------------------------------------

INSERT INTO roles (code, name, description) VALUES
    ('super_admin',      'Platform Super Admin', 'Full platform access'),
    ('tenant_owner',     'Tenant Owner',         'Full access within a tenant'),
    ('org_admin',        'Organization Admin',   'Manage one organization'),
    ('domain_admin',     'Domain Admin',         'Manage domains and mailboxes'),
    ('developer',        'Developer',            'API keys, webhooks, sending'),
    ('security_analyst', 'Security Analyst',     'Security center and audit'),
    ('auditor',          'Viewer/Auditor',       'Read-only audit access'),
    ('billing_admin',    'Billing Admin',        'Billing management (reserved)'),
    ('member',           'Member',               'Regular mailbox user');

INSERT INTO permissions (code, description) VALUES
    ('platform.manage',      'Manage platform-wide settings and tenants'),
    ('organizations.manage', 'Create and manage organizations'),
    ('users.manage',         'Create and manage users'),
    ('domains.manage',       'Create and manage domains'),
    ('mailboxes.manage',     'Create and manage mailboxes, aliases, groups'),
    ('apikeys.manage',       'Create and manage API keys and SMTP credentials'),
    ('webhooks.manage',      'Create and manage webhooks'),
    ('security.manage',      'Security center and quarantine actions'),
    ('audit.read',           'Read audit logs'),
    ('mail.send',            'Send mail'),
    ('mail.read',            'Read own mailboxes');

INSERT INTO role_permissions (role_code, permission_code)
SELECT 'super_admin', code FROM permissions;

INSERT INTO role_permissions (role_code, permission_code)
SELECT 'tenant_owner', code FROM permissions WHERE code <> 'platform.manage';

INSERT INTO role_permissions (role_code, permission_code) VALUES
    ('org_admin', 'organizations.manage'),
    ('org_admin', 'users.manage'),
    ('org_admin', 'domains.manage'),
    ('org_admin', 'mailboxes.manage'),
    ('org_admin', 'apikeys.manage'),
    ('org_admin', 'webhooks.manage'),
    ('org_admin', 'audit.read'),
    ('org_admin', 'mail.send'),
    ('org_admin', 'mail.read'),
    ('domain_admin', 'domains.manage'),
    ('domain_admin', 'mailboxes.manage'),
    ('developer', 'apikeys.manage'),
    ('developer', 'webhooks.manage'),
    ('developer', 'mail.send'),
    ('security_analyst', 'security.manage'),
    ('security_analyst', 'audit.read'),
    ('auditor', 'audit.read'),
    ('member', 'mail.send'),
    ('member', 'mail.read');

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS user_credentials;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS organizations;
DROP TABLE IF EXISTS tenants;
