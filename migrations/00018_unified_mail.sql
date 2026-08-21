-- +goose Up
-- Unified mail store (ADR-003):
--   1. 'relayed' recipient status: handed to the mail core's outbound queue
--      for a remote recipient.
--   2. messages.sent_imported_at guards the once-per-message Sent-copy
--      import into the mail store.
--   3. provisioning_jobs: asynchronous mail-core provisioning with retry,
--      replacing synchronous calls inside HTTP handlers.
--   4. mailbox_messages.migrated_at marks legacy rows backfilled into the
--      mail store (legacy tables stay read-only until migration is proven).

ALTER TABLE message_recipients DROP CONSTRAINT message_recipients_status_check;
ALTER TABLE message_recipients ADD CONSTRAINT message_recipients_status_check
    CHECK (status IN ('pending', 'delivered', 'relayed', 'failed', 'quarantined'));

ALTER TABLE messages ADD COLUMN sent_imported_at timestamptz;

CREATE TABLE provisioning_jobs (
    id              bigserial PRIMARY KEY,
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    kind            text NOT NULL CHECK (kind IN ('domain', 'mailbox')),
    entity_id       uuid NOT NULL,
    actor_user_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'running', 'done', 'failed')),
    attempts        int NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error      text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX provisioning_jobs_due_idx ON provisioning_jobs (next_attempt_at)
    WHERE status IN ('pending', 'running');
-- One live job per entity keeps retries and duplicate clicks idempotent.
CREATE UNIQUE INDEX provisioning_jobs_live_idx ON provisioning_jobs (kind, entity_id)
    WHERE status IN ('pending', 'running');

ALTER TABLE mailbox_messages ADD COLUMN migrated_at timestamptz;

-- +goose Down
ALTER TABLE mailbox_messages DROP COLUMN IF EXISTS migrated_at;
DROP TABLE IF EXISTS provisioning_jobs;
ALTER TABLE messages DROP COLUMN IF EXISTS sent_imported_at;
ALTER TABLE message_recipients DROP CONSTRAINT message_recipients_status_check;
ALTER TABLE message_recipients ADD CONSTRAINT message_recipients_status_check
    CHECK (status IN ('pending', 'delivered', 'failed', 'quarantined'));
