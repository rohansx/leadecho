-- name: CreateExtensionToken :one
INSERT INTO extension_tokens (workspace_id, token, name)
VALUES (@workspace_id, @token, @name)
RETURNING *;

-- name: GetExtensionTokenByToken :one
SELECT * FROM extension_tokens
WHERE token = @token;

-- name: GetExtensionTokenByWorkspace :one
SELECT * FROM extension_tokens
WHERE workspace_id = @workspace_id;

-- name: TouchExtensionToken :exec
UPDATE extension_tokens
SET last_used_at = NOW()
WHERE token = @token;

-- name: DeleteExtensionTokenByWorkspace :exec
DELETE FROM extension_tokens
WHERE workspace_id = @workspace_id;
