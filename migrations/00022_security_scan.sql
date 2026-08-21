-- +goose Up
-- Security scan metadata (V4 §41): the full verdict of the acceptance-path
-- scan (deterministic rules + Rspamd + ClamAV) is stored per message and
-- surfaced in Security Center / Message Trace detail views.
ALTER TABLE messages ADD COLUMN security_scan jsonb;

-- Quarantine lifecycle (§48) gains 'pending_scan': held because no malware
-- scanner was reachable, resolved automatically once scanning succeeds.
ALTER TABLE quarantine_items DROP CONSTRAINT IF EXISTS quarantine_items_status_check;
ALTER TABLE quarantine_items ADD CONSTRAINT quarantine_items_status_check
    CHECK (status IN ('pending', 'pending_scan', 'released', 'deleted'));

-- +goose Down
ALTER TABLE quarantine_items DROP CONSTRAINT IF EXISTS quarantine_items_status_check;
ALTER TABLE quarantine_items ADD CONSTRAINT quarantine_items_status_check
    CHECK (status IN ('pending', 'released', 'deleted'));
ALTER TABLE messages DROP COLUMN IF EXISTS security_scan;
