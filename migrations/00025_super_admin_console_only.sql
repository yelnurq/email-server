-- +goose Up
-- Keep platform super admins in the admin console only: they retain the
-- admin/operations/security permissions but lose direct mail access.

DELETE FROM role_permissions
WHERE role_code = 'super_admin'
  AND permission_code IN ('mail.read', 'mail.send');

DELETE FROM user_roles ur
USING user_roles sa
WHERE ur.user_id = sa.user_id
  AND sa.role_code = 'super_admin'
  AND ur.role_code = 'tenant_owner';

-- +goose Down
INSERT INTO role_permissions (role_code, permission_code) VALUES
    ('super_admin', 'mail.read'),
    ('super_admin', 'mail.send')
ON CONFLICT DO NOTHING;

INSERT INTO user_roles (user_id, role_code, scope_type, scope_id)
SELECT u.id, 'tenant_owner', 'tenant', u.tenant_id
FROM users u
WHERE EXISTS (
    SELECT 1 FROM user_roles ur
    WHERE ur.user_id = u.id AND ur.role_code = 'super_admin'
)
ON CONFLICT DO NOTHING;
