-- +goose Up
-- Full-text search over messages. The 'simple' configuration is
-- language-agnostic (no stemming), which fits mixed RU/EN corporate mail;
-- language-aware configs can be added later behind the search abstraction.

ALTER TABLE messages ADD COLUMN search_tsv tsvector
    GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(subject, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(body_text, '')), 'B')
    ) STORED;

CREATE INDEX messages_search_idx ON messages USING GIN (search_tsv);

-- +goose Down
DROP INDEX IF EXISTS messages_search_idx;
ALTER TABLE messages DROP COLUMN IF EXISTS search_tsv;
