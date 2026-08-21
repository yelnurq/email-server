-- +goose Up
-- DNS verification subsystem (V4 §11-16):
--   1. domain_dns_records holds the LATEST check result per record type —
--      an upsert target, deliberately not an unbounded history (§14).
--   2. domains.verification_token backs the ownership TXT record
--      (_mailplatform.<domain> TXT "mailplatform-verify=<token>").
--   3. dns_checked_at / verified_at stamp the last recheck and the moment a
--      dns-mode domain proved ownership.

CREATE TABLE domain_dns_records (
    domain_id   uuid NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    record_type text NOT NULL CHECK (record_type IN
                ('ownership', 'mx', 'spf', 'dkim', 'dmarc', 'mta_sts', 'tls_rpt', 'ptr')),
    host        text NOT NULL DEFAULT '',
    expected    text NOT NULL DEFAULT '',
    detected    jsonb NOT NULL DEFAULT '[]',
    status      text NOT NULL DEFAULT 'pending' CHECK (status IN
                ('verified', 'missing', 'invalid', 'warning', 'pending', 'dns_error')),
    detail      text NOT NULL DEFAULT '',
    checked_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (domain_id, record_type)
);

ALTER TABLE domains ADD COLUMN verification_token text NOT NULL DEFAULT '';
ALTER TABLE domains ADD COLUMN dns_checked_at timestamptz;
ALTER TABLE domains ADD COLUMN verified_at timestamptz;

-- +goose Down
ALTER TABLE domains DROP COLUMN IF EXISTS verified_at;
ALTER TABLE domains DROP COLUMN IF EXISTS dns_checked_at;
ALTER TABLE domains DROP COLUMN IF EXISTS verification_token;
DROP TABLE IF EXISTS domain_dns_records;
