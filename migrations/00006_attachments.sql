-- +goose Up
-- Attachments: metadata in PostgreSQL, content in S3-compatible storage.
-- Uploaded files are staged (message_id IS NULL) until a send links them.

CREATE TABLE attachments (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    public_id        text NOT NULL UNIQUE, -- att_..., safe for the API
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    message_id       uuid REFERENCES messages(id) ON DELETE CASCADE,
    uploader_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    storage_key      text NOT NULL,
    filename         text NOT NULL,
    content_type     text NOT NULL DEFAULT 'application/octet-stream',
    size_bytes       bigint NOT NULL CHECK (size_bytes >= 0),
    checksum_sha256  text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX attachments_message_idx ON attachments (message_id);
CREATE INDEX attachments_tenant_idx ON attachments (tenant_id);
-- staged uploads eligible for cleanup
CREATE INDEX attachments_staged_idx ON attachments (created_at) WHERE message_id IS NULL;

-- +goose Down
DROP TABLE IF EXISTS attachments;
