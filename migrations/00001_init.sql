-- +goose Up
-- Baseline migration: shared database extensions.
-- citext provides case-insensitive text, used later for email addresses and
-- domain names (mailbox local-parts and domains compare case-insensitively).
CREATE EXTENSION IF NOT EXISTS citext;

-- +goose Down
DROP EXTENSION IF EXISTS citext;
