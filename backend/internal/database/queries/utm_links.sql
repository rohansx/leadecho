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

-- name: GetUTMLinkByID :one
SELECT * FROM utm_links WHERE id = @id AND workspace_id = @workspace_id;

-- name: CreateUTMEvent :one
INSERT INTO utm_events (
    utm_link_id, event_type, referrer, user_agent, ip_hash, revenue_cents, metadata
) VALUES (
    @utm_link_id, @event_type, @referrer, @user_agent, @ip_hash, @revenue_cents, @metadata
) RETURNING *;

-- name: RecordUTMConversion :one
UPDATE utm_links
SET signup_count = signup_count + 1,
    revenue_cents = revenue_cents + @revenue_cents
WHERE code = @code
RETURNING *;
