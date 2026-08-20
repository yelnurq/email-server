-- +goose Up
-- Domains, mailboxes, folders and aliases.

CREATE TABLE domains (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    organization_id   uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name              citext NOT NULL UNIQUE, -- mail domains are globally unique
    status            text NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'verified', 'failed', 'disabled')),
    -- development: local-only domain, verification bypassed by design;
    -- dns: production path requiring real DNS verification (later phase).
    verification_mode text NOT NULL DEFAULT 'dns'
                      CHECK (verification_mode IN ('development', 'dns')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX domains_tenant_idx ON domains (tenant_id);
CREATE INDEX domains_org_idx ON domains (organization_id);

CREATE TABLE mailboxes (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    domain_id       uuid NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    user_id         uuid REFERENCES users(id) ON DELETE SET NULL, -- NULL: shared/service mailbox
    local_part      citext NOT NULL,
    address         citext NOT NULL UNIQUE, -- local_part@domain, denormalized for lookups
    status          text NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'suspended', 'disabled')),
    quota_bytes     bigint NOT NULL DEFAULT 1073741824, -- 1 GiB
    used_bytes      bigint NOT NULL DEFAULT 0 CHECK (used_bytes >= 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (domain_id, local_part)
);
CREATE INDEX mailboxes_tenant_idx ON mailboxes (tenant_id);
CREATE INDEX mailboxes_user_idx ON mailboxes (user_id);

CREATE TABLE folders (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    mailbox_id  uuid NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
    name        text NOT NULL,
    -- system folders are provisioned with the mailbox and cannot be removed
    type        text NOT NULL DEFAULT 'custom'
                CHECK (type IN ('inbox', 'sent', 'drafts', 'spam', 'trash', 'custom')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (mailbox_id, name)
);
-- exactly one system folder of each type per mailbox
CREATE UNIQUE INDEX folders_system_unique_idx ON folders (mailbox_id, type) WHERE type <> 'custom';

CREATE TABLE mailbox_aliases (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    domain_id       uuid NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    local_part      citext NOT NULL,
    address         citext NOT NULL UNIQUE,
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (domain_id, local_part)
);

CREATE TABLE mailbox_alias_targets (
    alias_id    uuid NOT NULL REFERENCES mailbox_aliases(id) ON DELETE CASCADE,
    mailbox_id  uuid NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
    PRIMARY KEY (alias_id, mailbox_id)
);

-- +goose Down
DROP TABLE IF EXISTS mailbox_alias_targets;
DROP TABLE IF EXISTS mailbox_aliases;
DROP TABLE IF EXISTS folders;
DROP TABLE IF EXISTS mailboxes;
DROP TABLE IF EXISTS domains;
