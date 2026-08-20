-- +goose Up
CREATE TABLE user_presence (
 user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
 organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 online boolean NOT NULL DEFAULT false,
 last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX user_presence_org_idx ON user_presence(tenant_id,organization_id,online,last_seen_at DESC);
-- +goose Down
DROP TABLE IF EXISTS user_presence;
