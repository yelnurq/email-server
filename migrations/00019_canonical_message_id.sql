-- +goose Up
-- Canonical Message-ID form (V4 data integrity, §5):
--
-- The platform now stores RFC Message-IDs bare — no angle brackets, domain
-- lowercased — matching what JMAP reports, so cross-store comparisons are
-- plain equality. Brackets are wire format, added only by the MIME
-- renderer. The old bracketed storage made two webmail queries dead code
-- and caused the V3 duplicate-import incident.
--
-- lower() over the whole value is safe here even though RFC 5322 id-left is
-- case-sensitive: every row in these columns is platform-generated
-- (lowercase uuid @ lowercase domain), so this is the identity transform on
-- real data. Foreign ids ingested after V4 are normalized in Go, which
-- lowercases only the domain half.

UPDATE messages SET
    rfc_message_id = lower(btrim(replace(replace(rfc_message_id, '<', ''), '>', ''))),
    in_reply_to    = lower(btrim(replace(replace(in_reply_to,    '<', ''), '>', ''))),
    references_ids = lower(btrim(regexp_replace(
                         replace(replace(references_ids, '<', ''), '>', ''),
                         '\s+', ' ', 'g')))
WHERE rfc_message_id <> '' OR in_reply_to <> '' OR references_ids <> '';

-- The pending-migration scan (cmd/migrate-mail) filters on migrated_at IS
-- NULL; give it a partial index instead of a sequential scan.
CREATE INDEX mailbox_messages_unmigrated_idx ON mailbox_messages (mailbox_id)
    WHERE migrated_at IS NULL;

-- +goose Down
-- The canonical form is valid input for every consumer (the normalizer is
-- idempotent), so restoring brackets is unnecessary; only the index is
-- rolled back.
DROP INDEX IF EXISTS mailbox_messages_unmigrated_idx;
