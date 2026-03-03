-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id = @id;

-- name: GetWorkspaceBySlug :one
SELECT * FROM workspaces WHERE slug = @slug;

-- name: CreateWorkspace :one
INSERT INTO workspaces (clerk_org_id, name, slug)
VALUES (@clerk_org_id, @name, @slug)
RETURNING *;

-- name: GetWorkspaceSettings :one
SELECT settings FROM workspaces WHERE id = @id;

-- name: UpdateWorkspaceSettings :exec
UPDATE workspaces SET settings = @settings WHERE id = @id;
