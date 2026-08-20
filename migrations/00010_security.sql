-- +goose Up
-- Security baseline: sender blocks, quarantine, incidents; extended
-- recipient/message statuses for quarantined mail.

CREATE TABLE sender_blocks (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    -- exact address ("user@evil.test") or whole domain ("evil.test")
    pattern    citext NOT NULL,
    kind       text NOT NULL CHECK (kind IN ('address', 'domain')),
    reason     text NOT NULL DEFAULT '',
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, pattern)
);

CREATE TABLE quarantine_items (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    message_id        uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    recipient_id      uuid NOT NULL REFERENCES message_recipients(id) ON DELETE CASCADE,
    recipient_address citext NOT NULL,
    mailbox_id        uuid REFERENCES mailboxes(id) ON DELETE SET NULL,
    reason            text NOT NULL,
    signals           jsonb NOT NULL DEFAULT '[]',
    risk_score        int NOT NULL DEFAULT 0,
    status            text NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'released', 'deleted')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    resolved_at       timestamptz,
    resolved_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE (message_id, recipient_id)
);
CREATE INDEX quarantine_tenant_idx ON quarantine_items (tenant_id, status, created_at DESC);

CREATE TABLE security_incidents (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kind       text NOT NULL,
    detail     jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX security_incidents_tenant_idx ON security_incidents (tenant_id, created_at DESC);

ALTER TABLE message_recipients DROP CONSTRAINT message_recipients_status_check;
ALTER TABLE message_recipients ADD CONSTRAINT message_recipients_status_check
    CHECK (status IN ('pending', 'delivered', 'failed', 'quarantined'));

ALTER TABLE messages DROP CONSTRAINT messages_status_check;
ALTER TABLE messages ADD CONSTRAINT messages_status_check
    CHECK (status IN ('draft', 'accepted', 'processing', 'delivered',
                      'partially_delivered', 'failed', 'quarantined'));

-- +goose Down
ALTER TABLE messages DROP CONSTRAINT messages_status_check;
ALTER TABLE messages ADD CONSTRAINT messages_status_check
    CHECK (status IN ('draft', 'accepted', 'processing', 'delivered',
                      'partially_delivered', 'failed'));
ALTER TABLE message_recipients DROP CONSTRAINT message_recipients_status_check;
ALTER TABLE message_recipients ADD CONSTRAINT message_recipients_status_check
    CHECK (status IN ('pending', 'delivered', 'failed'));
DROP TABLE IF EXISTS security_incidents;
DROP TABLE IF EXISTS quarantine_items;
DROP TABLE IF EXISTS sender_blocks;
