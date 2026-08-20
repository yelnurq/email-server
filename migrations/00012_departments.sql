-- +goose Up
-- Organization-scoped departments and user membership.

CREATE TABLE departments (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    organization_id   uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name              text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    description       text NOT NULL DEFAULT '' CHECK (char_length(description) <= 1000),
    manager_user_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX departments_org_name_unique
    ON departments (organization_id, lower(name));
CREATE INDEX departments_tenant_org_idx
    ON departments (tenant_id, organization_id, name);

ALTER TABLE users
    ADD COLUMN department_id uuid REFERENCES departments(id) ON DELETE SET NULL;
CREATE INDEX users_department_idx ON users (department_id) WHERE department_id IS NOT NULL;

-- Manager and department must belong to the same tenant and organization.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_department_manager_scope()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.manager_user_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM users u
        WHERE u.id = NEW.manager_user_id
          AND u.tenant_id = NEW.tenant_id
          AND u.organization_id = NEW.organization_id
    ) THEN
        RAISE EXCEPTION 'department manager must belong to the same organization';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER departments_manager_scope
BEFORE INSERT OR UPDATE OF manager_user_id, organization_id, tenant_id ON departments
FOR EACH ROW EXECUTE FUNCTION validate_department_manager_scope();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION validate_user_department_scope()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.department_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM departments d
        WHERE d.id = NEW.department_id
          AND d.tenant_id = NEW.tenant_id
          AND d.organization_id = NEW.organization_id
    ) THEN
        RAISE EXCEPTION 'user department must belong to the same organization';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER users_department_scope
BEFORE INSERT OR UPDATE OF department_id, organization_id, tenant_id ON users
FOR EACH ROW EXECUTE FUNCTION validate_user_department_scope();

-- +goose Down
DROP TRIGGER IF EXISTS users_department_scope ON users;
DROP FUNCTION IF EXISTS validate_user_department_scope();
DROP TRIGGER IF EXISTS departments_manager_scope ON departments;
DROP FUNCTION IF EXISTS validate_department_manager_scope();
DROP INDEX IF EXISTS users_department_idx;
ALTER TABLE users DROP COLUMN IF EXISTS department_id;
DROP TABLE IF EXISTS departments;
