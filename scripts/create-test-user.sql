-- Create test user for login testing
-- Usage: psql -d email_server -f scripts/create-test-user.sql

-- Get the first tenant and organization
WITH first_tenant AS (
  SELECT id FROM tenants LIMIT 1
),
first_org AS (
  SELECT id FROM organizations LIMIT 1
),
user_data AS (
  INSERT INTO users (tenant_id, organization_id, email, display_name)
  VALUES ((SELECT id FROM first_tenant), (SELECT id FROM first_org), 'user1@company.test', 'Test User')
  ON CONFLICT (email) DO NOTHING
  RETURNING id, tenant_id
)
-- Insert credentials
INSERT INTO user_credentials (user_id, password_hash)
SELECT ud.id, '$argon2id$v=19$m=65536,t=3,p=4$gNZM8U5J7tNuJZZ8bNzFzA$7Q+ycz5VXXbN7q/N2ZmFUXqHmB7q7X3J7X3J7X3J7N8' 
FROM user_data ud
ON CONFLICT (user_id) DO NOTHING;

-- Grant 'member' role
INSERT INTO user_roles (user_id, role_code, scope_type, scope_id)
SELECT ud.id, 'member', 'organization', (SELECT id FROM first_org)
FROM user_data ud
ON CONFLICT DO NOTHING;

SELECT 'Test user created or already exists' as result;
