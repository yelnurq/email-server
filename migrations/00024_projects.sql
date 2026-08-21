-- +goose Up
-- Projects (V4 §79-84): the scope layer between Organization and mail
-- infrastructure. Organization → Project → Domain; API keys and SMTP
-- credentials attach to a project. Departments stay a people concept under
-- Organization (§83) — unrelated to projects.

CREATE TABLE projects (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            text NOT NULL,
    slug            citext NOT NULL,
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, slug)
);
CREATE INDEX projects_org_idx ON projects (organization_id);

ALTER TABLE domains ADD COLUMN project_id uuid REFERENCES projects(id) ON DELETE SET NULL;
ALTER TABLE api_keys ADD COLUMN project_id uuid REFERENCES projects(id) ON DELETE SET NULL;
ALTER TABLE smtp_credentials ADD COLUMN project_id uuid REFERENCES projects(id) ON DELETE SET NULL;
CREATE INDEX domains_project_idx ON domains (project_id);

-- Default migration (§81): every existing organization gets a Default
-- project and its existing resources attach to it — no manual data fixing.
INSERT INTO projects (tenant_id, organization_id, name, slug)
SELECT tenant_id, id, 'Default', 'default' FROM organizations;

UPDATE domains d SET project_id = p.id
FROM projects p
WHERE p.organization_id = d.organization_id AND p.slug = 'default' AND d.project_id IS NULL;

UPDATE api_keys k SET project_id = p.id
FROM projects p
WHERE p.organization_id = k.organization_id AND p.slug = 'default' AND k.project_id IS NULL;

UPDATE smtp_credentials c SET project_id = p.id
FROM projects p
WHERE p.organization_id = c.organization_id AND p.slug = 'default' AND c.project_id IS NULL;

-- +goose Down
ALTER TABLE smtp_credentials DROP COLUMN IF EXISTS project_id;
ALTER TABLE api_keys DROP COLUMN IF EXISTS project_id;
ALTER TABLE domains DROP COLUMN IF EXISTS project_id;
DROP TABLE IF EXISTS projects;
