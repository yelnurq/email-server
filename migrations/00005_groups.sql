-- +goose Up
-- Mail groups: one address delivering to many member mailboxes.
-- Loop protection is structural: aliases and groups target mailboxes only
-- (no nesting), so resolution depth is always 1.

CREATE TABLE mail_groups (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    domain_id       uuid NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    local_part      citext NOT NULL,
    address         citext NOT NULL UNIQUE,
    name            text NOT NULL DEFAULT '',
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    -- internal_only: only senders from the same organization may deliver
    internal_only   boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (domain_id, local_part)
);
CREATE INDEX mail_groups_tenant_idx ON mail_groups (tenant_id);

CREATE TABLE mail_group_members (
    group_id   uuid NOT NULL REFERENCES mail_groups(id) ON DELETE CASCADE,
    mailbox_id uuid NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, mailbox_id)
);

-- +goose Down
DROP TABLE IF EXISTS mail_group_members;
DROP TABLE IF EXISTS mail_groups;
