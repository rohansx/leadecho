-- name: CreateUTMLink :one
INSERT INTO utm_links (workspace_id, code, destination_url, utm_source, utm_medium, utm_campaign, utm_content)
VALUES (@workspace_id, @code, @destination_url, @utm_source, @utm_medium, @utm_campaign, @utm_content)
RETURNING *;

-- name: ListUTMLinksByWorkspace :many
SELECT * FROM utm_links WHERE workspace_id = @workspace_id ORDER BY created_at DESC;

-- name: GetUTMLinkByCode :one
SELECT * FROM utm_links WHERE code = @code;

-- name: IncrementUTMClicks :exec
UPDATE utm_links SET click_count = click_count + 1 WHERE code = @code;

-- name: DeleteUTMLink :exec
DELETE FROM utm_links WHERE id = @id AND workspace_id = @workspace_id;
