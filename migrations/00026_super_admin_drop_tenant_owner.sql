-- +goose Up
-- Ensure platform super admins are console-only by removing the tenant-owner
-- grant that would otherwise reintroduce mail access.

DELETE FROM user_roles ur
WHERE ur.role_code = 'tenant_owner'
  AND EXISTS (
      SELECT 1
      FROM user_roles sa
      WHERE sa.user_id = ur.user_id
        AND sa.role_code = 'super_admin'
  );

-- +goose Down
INSERT INTO user_roles (user_id, role_code, scope_type, scope_id)
SELECT u.id, 'tenant_owner', 'tenant', u.tenant_id
FROM users u
WHERE EXISTS (
    SELECT 1 FROM user_roles ur
    WHERE ur.user_id = u.id AND ur.role_code = 'super_admin'
)
ON CONFLICT DO NOTHING;
