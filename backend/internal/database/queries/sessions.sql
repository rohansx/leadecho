-- name: GetPlatformSession :one
SELECT * FROM platform_accounts
WHERE workspace_id = @workspace_id
  AND platform = @platform::platform_type
LIMIT 1;

-- name: UpsertPlatformSession :one
INSERT INTO platform_accounts (workspace_id, user_id, platform, username, access_token_enc, is_active, metadata)
VALUES (@workspace_id, @user_id, @platform::platform_type, @username, @access_token_enc, true, @metadata)
ON CONFLICT (workspace_id, user_id, platform)
DO UPDATE SET
    access_token_enc = EXCLUDED.access_token_enc,
    username         = EXCLUDED.username,
    is_active        = true,
    metadata         = EXCLUDED.metadata,
    updated_at       = NOW()
RETURNING *;

-- name: DeletePlatformSession :exec
DELETE FROM platform_accounts
WHERE workspace_id = @workspace_id
  AND platform = @platform::platform_type;

-- name: ListPlatformSessions :many
SELECT * FROM platform_accounts
WHERE workspace_id = @workspace_id
  AND is_active = true
ORDER BY platform;
