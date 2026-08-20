-- +goose Up
ALTER TABLE chat_messages ALTER COLUMN attachment_ids TYPE text[] USING attachment_ids::text[];
ALTER TABLE attachments ADD COLUMN chat_message_id uuid REFERENCES chat_messages(id) ON DELETE CASCADE;
CREATE INDEX attachments_chat_message_idx ON attachments(chat_message_id) WHERE chat_message_id IS NOT NULL;
-- One staged attachment can be linked to email or chat, never both.
ALTER TABLE attachments ADD CONSTRAINT attachments_single_parent CHECK (message_id IS NULL OR chat_message_id IS NULL);

-- +goose Down
ALTER TABLE attachments DROP CONSTRAINT IF EXISTS attachments_single_parent;
DROP INDEX IF EXISTS attachments_chat_message_idx;
ALTER TABLE attachments DROP COLUMN IF EXISTS chat_message_id;
ALTER TABLE chat_messages ALTER COLUMN attachment_ids TYPE uuid[] USING attachment_ids::uuid[];
