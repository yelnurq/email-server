-- +goose Up
ALTER TABLE bulk_email_recipients ADD COLUMN message_public_id text;
CREATE INDEX bulk_recipients_message_idx ON bulk_email_recipients(message_public_id) WHERE message_public_id IS NOT NULL;
-- +goose Down
DROP INDEX IF EXISTS bulk_recipients_message_idx;
ALTER TABLE bulk_email_recipients DROP COLUMN IF EXISTS message_public_id;
