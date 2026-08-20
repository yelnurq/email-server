-- +goose Up
-- Mail-core integration groundwork:
--   1. Self-mail fix: one message may exist in several folders of the same
--      mailbox (Sent + Inbox for A→A mail). The old UNIQUE(mailbox_id,
--      message_id) made the Sent copy shadow the Inbox delivery.
--   2. Provisioning lifecycle columns for domains and mailboxes so a row in
--      PostgreSQL is not silently equated with a provisioned entity in the
--      mail core (Stalwart).
--   3. Worker heartbeats so the control plane can report worker liveness
--      from a real signal instead of guessing.

ALTER TABLE mailbox_messages
    DROP CONSTRAINT mailbox_messages_mailbox_id_message_id_key;
ALTER TABLE mailbox_messages
    ADD CONSTRAINT mailbox_messages_copy_key UNIQUE (mailbox_id, message_id, folder_id);

-- pending:      created locally, mail core not yet attempted
-- provisioning: attempt in flight
-- active:       confirmed in the mail core
-- failed:       last attempt failed (see provisioning_error); retryable
-- skipped:      mail core disabled (MAIL_CORE_PROVIDER=none)
ALTER TABLE domains
    ADD COLUMN provisioning_status text NOT NULL DEFAULT 'pending'
        CHECK (provisioning_status IN ('pending', 'provisioning', 'active', 'failed', 'skipped')),
    ADD COLUMN provisioning_error  text NOT NULL DEFAULT '',
    ADD COLUMN provisioned_at      timestamptz;

ALTER TABLE mailboxes
    ADD COLUMN provisioning_status text NOT NULL DEFAULT 'pending'
        CHECK (provisioning_status IN ('pending', 'provisioning', 'active', 'failed', 'skipped')),
    ADD COLUMN provisioning_error  text NOT NULL DEFAULT '',
    ADD COLUMN provisioned_at      timestamptz;

-- One row per worker process kind; updated every few seconds while running.
CREATE TABLE worker_heartbeats (
    name        text PRIMARY KEY,
    beat_at     timestamptz NOT NULL DEFAULT now(),
    started_at  timestamptz NOT NULL DEFAULT now(),
    hostname    text NOT NULL DEFAULT ''
);

-- Message Trace queries: search accepted mail by sender and time window.
CREATE INDEX messages_trace_idx ON messages (tenant_id, created_at DESC)
    WHERE status <> 'draft';
CREATE INDEX message_recipients_address_idx ON message_recipients (address);

-- +goose Down
DROP INDEX IF EXISTS message_recipients_address_idx;
DROP INDEX IF EXISTS messages_trace_idx;
DROP TABLE IF EXISTS worker_heartbeats;
ALTER TABLE mailboxes
    DROP COLUMN IF EXISTS provisioned_at,
    DROP COLUMN IF EXISTS provisioning_error,
    DROP COLUMN IF EXISTS provisioning_status;
ALTER TABLE domains
    DROP COLUMN IF EXISTS provisioned_at,
    DROP COLUMN IF EXISTS provisioning_error,
    DROP COLUMN IF EXISTS provisioning_status;
ALTER TABLE mailbox_messages DROP CONSTRAINT mailbox_messages_copy_key;
-- Best-effort restore; duplicates created while the relaxed constraint was
-- active must be resolved manually before this succeeds.
ALTER TABLE mailbox_messages
    ADD CONSTRAINT mailbox_messages_mailbox_id_message_id_key UNIQUE (mailbox_id, message_id);
