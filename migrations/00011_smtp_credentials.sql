-- +goose Up
-- SMTP submission credentials. The SMTP listener itself arrives with the
-- mail-core integration phase; the credential lifecycle (issue/rotate/revoke,
-- hashed secrets, audit) is platform functionality and lands now.

CREATE TABLE smtp_credentials (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    mailbox_id      uuid NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
    username        citext NOT NULL UNIQUE,
    -- sha256 of the generated password; shown exactly once
    secret_hash     bytea NOT NULL,
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    last_used_at    timestamptz,
    created_by      uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX smtp_credentials_tenant_idx ON smtp_credentials (tenant_id);
CREATE INDEX smtp_credentials_mailbox_idx ON smtp_credentials (mailbox_id);

-- +goose Down
DROP TABLE IF EXISTS smtp_credentials;
