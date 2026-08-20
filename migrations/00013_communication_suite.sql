-- +goose Up
-- Collaboration suite. Chat entities are prefixed to avoid colliding with
-- the existing RFC email messages/threads model.

INSERT INTO permissions (code, description) VALUES
 ('departments.read', 'Read departments and organization directory'),
 ('departments.manage', 'Manage departments and membership'),
 ('messages.send', 'Use corporate messenger'),
 ('messages.group.create', 'Create group conversations'),
 ('official.read', 'Read official messages'),
 ('official.send.department', 'Send official messages to a department'),
 ('official.send.organization', 'Send official messages to an organization'),
 ('tasks.manage.self', 'Manage personal tasks and reminders'),
 ('tasks.assign.department', 'Assign reminders within a managed department'),
 ('bulk_email.create', 'Create bulk email campaigns'),
 ('bulk_email.send', 'Send or schedule bulk email campaigns'),
 ('bulk_email.view_analytics', 'View bulk email analytics')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_code, permission_code)
SELECT r.code, p.code FROM roles r CROSS JOIN permissions p
WHERE r.code IN ('super_admin','tenant_owner') AND p.code IN
 ('departments.read','departments.manage','messages.send','messages.group.create','official.read','official.send.department','official.send.organization','tasks.manage.self','tasks.assign.department','bulk_email.create','bulk_email.send','bulk_email.view_analytics')
ON CONFLICT DO NOTHING;
INSERT INTO role_permissions (role_code, permission_code) VALUES
 ('org_admin','departments.read'),('org_admin','departments.manage'),('org_admin','messages.send'),('org_admin','messages.group.create'),('org_admin','official.read'),('org_admin','official.send.department'),('org_admin','official.send.organization'),('org_admin','tasks.manage.self'),('org_admin','tasks.assign.department'),('org_admin','bulk_email.create'),('org_admin','bulk_email.send'),('org_admin','bulk_email.view_analytics'),
 ('member','departments.read'),('member','messages.send'),('member','official.read'),('member','tasks.manage.self'),
 ('domain_admin','departments.read'),('domain_admin','messages.send'),('domain_admin','official.read'),('domain_admin','tasks.manage.self'),
 ('developer','departments.read'),('developer','messages.send'),('developer','official.read'),('developer','tasks.manage.self'),
 ('security_analyst','departments.read'),('security_analyst','messages.send'),('security_analyst','official.read'),('security_analyst','tasks.manage.self')
ON CONFLICT DO NOTHING;

CREATE TABLE chat_conversations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
 organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK (kind IN ('direct','group')), title text NOT NULL DEFAULT '',
 created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX chat_conversations_scope_idx ON chat_conversations (tenant_id, organization_id, updated_at DESC);
CREATE TABLE chat_conversation_members (
 conversation_id uuid NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, role text NOT NULL DEFAULT 'member' CHECK (role IN ('owner','member')),
 last_read_at timestamptz, joined_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY (conversation_id,user_id)
);
CREATE INDEX chat_members_user_idx ON chat_conversation_members (user_id, conversation_id);
CREATE TABLE chat_messages (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), conversation_id uuid NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
 sender_user_id uuid REFERENCES users(id) ON DELETE SET NULL, reply_to_id uuid REFERENCES chat_messages(id) ON DELETE SET NULL,
 body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 10000), attachment_ids uuid[] NOT NULL DEFAULT '{}',
 edited_at timestamptz, deleted_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX chat_messages_conversation_idx ON chat_messages (conversation_id, created_at DESC);
CREATE TABLE chat_message_reactions (
 message_id uuid NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE, user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 emoji text NOT NULL CHECK (char_length(emoji) BETWEEN 1 AND 32), created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(message_id,user_id,emoji)
);

CREATE TABLE notifications (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
 organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE, user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 kind text NOT NULL, title text NOT NULL, body text NOT NULL DEFAULT '', target_url text NOT NULL DEFAULT '',
 read_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);

CREATE TABLE official_messages (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
 organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE, sender_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 sender_role text NOT NULL DEFAULT '', title text NOT NULL, body text NOT NULL, requires_acknowledgement boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE official_message_recipients (
 official_message_id uuid NOT NULL REFERENCES official_messages(id) ON DELETE CASCADE, user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 delivered_at timestamptz NOT NULL DEFAULT now(), read_at timestamptz, acknowledged_at timestamptz, PRIMARY KEY(official_message_id,user_id)
);
CREATE INDEX official_recipients_user_idx ON official_message_recipients(user_id, delivered_at DESC);

CREATE TABLE tasks (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
 organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE, owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 assigned_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL, title text NOT NULL, description text NOT NULL DEFAULT '',
 due_at timestamptz, priority text NOT NULL DEFAULT 'normal' CHECK(priority IN ('low','normal','high','urgent')),
 status text NOT NULL DEFAULT 'todo' CHECK(status IN ('todo','in_progress','done')),
 source_type text NOT NULL DEFAULT 'manual' CHECK(source_type IN ('manual','email','chat','official')), source_id uuid,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tasks_owner_idx ON tasks(owner_user_id,status,due_at);
CREATE TABLE reminders (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
 organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE, user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, task_id uuid REFERENCES tasks(id) ON DELETE CASCADE,
 source_type text NOT NULL DEFAULT 'manual', source_id uuid, title text NOT NULL, remind_at timestamptz NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','completed','overdue')), notified_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX reminders_due_idx ON reminders(status,remind_at) WHERE status='pending';

CREATE TABLE bulk_email_campaigns (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
 organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE, created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 subject text NOT NULL, body_text text NOT NULL DEFAULT '', body_html text NOT NULL DEFAULT '',
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','scheduled','queued','sending','completed','failed')),
 department_ids uuid[] NOT NULL DEFAULT '{}', user_ids uuid[] NOT NULL DEFAULT '{}', whole_organization boolean NOT NULL DEFAULT false,
 scheduled_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX bulk_campaigns_scope_idx ON bulk_email_campaigns(tenant_id,organization_id,created_at DESC);
CREATE TABLE bulk_email_recipients (
 campaign_id uuid NOT NULL REFERENCES bulk_email_campaigns(id) ON DELETE CASCADE, user_id uuid REFERENCES users(id) ON DELETE SET NULL,
 address citext NOT NULL, department_id uuid REFERENCES departments(id) ON DELETE SET NULL,
 status text NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','sent','delivered','deferred','bounced','failed','read')),
 error text NOT NULL DEFAULT '', updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(campaign_id,address)
);

-- +goose Down
DROP TABLE IF EXISTS bulk_email_recipients; DROP TABLE IF EXISTS bulk_email_campaigns;
DROP TABLE IF EXISTS reminders; DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS official_message_recipients; DROP TABLE IF EXISTS official_messages;
DROP TABLE IF EXISTS notifications; DROP TABLE IF EXISTS chat_message_reactions; DROP TABLE IF EXISTS chat_messages;
DROP TABLE IF EXISTS chat_conversation_members; DROP TABLE IF EXISTS chat_conversations;
DELETE FROM role_permissions WHERE permission_code IN ('departments.read','departments.manage','messages.send','messages.group.create','official.read','official.send.department','official.send.organization','tasks.manage.self','tasks.assign.department','bulk_email.create','bulk_email.send','bulk_email.view_analytics');
DELETE FROM permissions WHERE code IN ('departments.read','departments.manage','messages.send','messages.group.create','official.read','official.send.department','official.send.organization','tasks.manage.self','tasks.assign.department','bulk_email.create','bulk_email.send','bulk_email.view_analytics');
