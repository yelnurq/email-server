-- +goose Up
-- Deliverability analytics (V4 §67-76) query support: time-window scans
-- over events and recipient outcomes (§157 — indexed, not seq-scanned).
CREATE INDEX message_events_time_type_idx ON message_events (created_at, type);
CREATE INDEX message_recipients_created_idx ON message_recipients (created_at);

-- +goose Down
DROP INDEX IF EXISTS message_recipients_created_idx;
DROP INDEX IF EXISTS message_events_time_type_idx;
