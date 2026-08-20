-- +goose Up
-- Message model: logical Message, per-mailbox MailboxMessage state, threads,
-- recipients, delivery events and the transactional outbox for NATS.

CREATE TABLE threads (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    subject    text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX threads_tenant_idx ON threads (tenant_id);

CREATE TABLE messages (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- public API identifier; sequential DB ids are never exposed
    public_id       text NOT NULL UNIQUE,
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    thread_id       uuid REFERENCES threads(id) ON DELETE SET NULL,
    -- RFC 5322 interoperability identifiers
    rfc_message_id  text NOT NULL DEFAULT '',
    in_reply_to     text NOT NULL DEFAULT '',
    references_ids  text NOT NULL DEFAULT '',
    from_address    citext NOT NULL,
    from_display    text NOT NULL DEFAULT '',
    subject         text NOT NULL DEFAULT '',
    snippet         text NOT NULL DEFAULT '', -- plain-text preview, capped
    body_text       text NOT NULL DEFAULT '',
    body_html       text NOT NULL DEFAULT '',
    has_attachments boolean NOT NULL DEFAULT false,
    size_bytes      bigint NOT NULL DEFAULT 0,
    status          text NOT NULL DEFAULT 'accepted'
                    CHECK (status IN ('draft', 'accepted', 'processing',
                                      'delivered', 'partially_delivered', 'failed')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    sent_at         timestamptz
);
CREATE INDEX messages_tenant_idx ON messages (tenant_id);
CREATE INDEX messages_thread_idx ON messages (thread_id);
CREATE UNIQUE INDEX messages_rfc_id_idx ON messages (rfc_message_id) WHERE rfc_message_id <> '';

CREATE TABLE message_recipients (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    kind       text NOT NULL CHECK (kind IN ('to', 'cc', 'bcc')),
    address    citext NOT NULL,
    -- resolved local mailbox; NULL until routed (or when remote/unknown)
    mailbox_id uuid REFERENCES mailboxes(id) ON DELETE SET NULL,
    status     text NOT NULL DEFAULT 'pending'
               CHECK (status IN ('pending', 'delivered', 'failed')),
    error      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX message_recipients_message_idx ON message_recipients (message_id);

CREATE TABLE mailbox_messages (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    mailbox_id  uuid NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
    message_id  uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    folder_id   uuid NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    is_read     boolean NOT NULL DEFAULT false,
    is_starred  boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    -- one copy of a message per mailbox; makes redelivery idempotent
    UNIQUE (mailbox_id, message_id)
);
CREATE INDEX mailbox_messages_list_idx ON mailbox_messages (mailbox_id, folder_id, created_at DESC);
CREATE INDEX mailbox_messages_message_idx ON mailbox_messages (message_id);

CREATE TABLE message_events (
    id         bigserial PRIMARY KEY,
    message_id uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    type       text NOT NULL, -- email.accepted, email.delivered_local, email.failed, ...
    detail     jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX message_events_message_idx ON message_events (message_id, created_at);

-- Transactional outbox: events are written in the same transaction as state
-- changes and published to NATS by a background publisher, closing the
-- DB -> broker consistency gap.
CREATE TABLE outbox (
    id           bigserial PRIMARY KEY,
    subject      text NOT NULL, -- NATS subject
    payload      jsonb NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);
CREATE INDEX outbox_unpublished_idx ON outbox (id) WHERE published_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS message_events;
DROP TABLE IF EXISTS mailbox_messages;
DROP TABLE IF EXISTS message_recipients;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS threads;
