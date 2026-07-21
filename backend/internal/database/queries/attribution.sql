-- name: UpdateReplyMetadata :one
UPDATE replies
SET metadata = @metadata
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: ReplyStyleAttribution :many
SELECT
    COALESCE(r.template_style, 'unknown') AS template_style,
    COUNT(*)::int AS reply_count,
    COUNT(*) FILTER (WHERE r.status = 'posted')::int AS posted_count,
    COALESCE(SUM(u.click_count), 0)::int AS click_count,
    COALESCE(SUM(u.signup_count), 0)::int AS signup_count
FROM replies r
LEFT JOIN utm_links u ON r.utm_link_id = u.id
WHERE r.workspace_id = @workspace_id
  AND r.created_at >= NOW() - INTERVAL '30 days'
GROUP BY 1
ORDER BY signup_count DESC, click_count DESC;
