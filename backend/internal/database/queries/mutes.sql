-- name: ListMutes :many
SELECT * FROM mutes
WHERE workspace_id = @workspace_id
ORDER BY created_at DESC;

-- name: CreateMute :one
-- Idempotent: re-muting the same value returns the existing row.
INSERT INTO mutes (workspace_id, kind, value, created_by)
VALUES (@workspace_id, @kind, @value, sqlc.narg('created_by'))
ON CONFLICT (workspace_id, kind, lower(value)) DO UPDATE SET value = mutes.value
RETURNING *;

-- name: DeleteMute :execrows
DELETE FROM mutes WHERE id = @id AND workspace_id = @workspace_id;
