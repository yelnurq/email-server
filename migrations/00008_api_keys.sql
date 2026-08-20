-- +goose Up
-- API keys for the Email API and the idempotency ledger for sends.

CREATE TABLE api_keys (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            text NOT NULL,
    -- first characters of the key, shown in the UI for identification
    prefix          text NOT NULL,
    -- sha256 of the full secret; the raw key is shown exactly once
    secret_hash     bytea NOT NULL UNIQUE,
    scopes          text[] NOT NULL DEFAULT '{emails.send}',
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    expires_at      timestamptz,
    last_used_at    timestamptz,
    created_by      uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX api_keys_tenant_idx ON api_keys (tenant_id);

CREATE TABLE idempotency_keys (
    id           bigserial PRIMARY KEY,
    api_key_id   uuid NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    idem_key     text NOT NULL,
    request_hash text NOT NULL, -- sha256 of the canonical payload
    message_id   text NOT NULL, -- public message id of the original send
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (api_key_id, idem_key)
);

-- Messages carry their sender's organization so API keys (org-scoped) can
-- read exactly the mail sent from their organization.
ALTER TABLE messages ADD COLUMN organization_id uuid REFERENCES organizations(id) ON DELETE SET NULL;
CREATE INDEX messages_org_idx ON messages (organization_id);

-- +goose Down
ALTER TABLE messages DROP COLUMN IF EXISTS organization_id;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS api_keys;
