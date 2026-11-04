-- name: CreateWorkspaceAPIKey :one
INSERT INTO workspace_api_keys (workspace_id, name, prefix, key_hash, scopes, created_by)
VALUES (@workspace_id, @name, @prefix, @key_hash, @scopes, @created_by)
RETURNING *;

-- name: ListWorkspaceAPIKeys :many
SELECT * FROM workspace_api_keys
WHERE workspace_id = @workspace_id
ORDER BY created_at DESC;

-- name: GetActiveWorkspaceAPIKeyByHash :one
SELECT * FROM workspace_api_keys
WHERE key_hash = @key_hash AND revoked_at IS NULL;

-- name: RevokeWorkspaceAPIKey :one
UPDATE workspace_api_keys
SET revoked_at = COALESCE(revoked_at, NOW())
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: TouchWorkspaceAPIKey :exec
UPDATE workspace_api_keys
SET last_used_at = NOW()
WHERE id = @id AND workspace_id = @workspace_id;
