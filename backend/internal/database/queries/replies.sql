-- name: ListRepliesByMention :many
SELECT * FROM replies
WHERE mention_id = @mention_id AND workspace_id = @workspace_id
ORDER BY created_at DESC;

-- name: GetReply :one
SELECT * FROM replies
WHERE id = @id AND workspace_id = @workspace_id;

-- name: CreateReply :one
INSERT INTO replies (
    mention_id, workspace_id, content, status, template_style, thread_context_used
) VALUES (
    @mention_id, @workspace_id, @content, @status, @template_style, @thread_context_used
) RETURNING *;

-- name: UpdateReplyContent :one
UPDATE replies
SET edited_content = @edited_content
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: UpdateReplyStatus :one
UPDATE replies
SET status = @status
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: CountRepliesByStatus :many
SELECT status, COUNT(*)::int as count
FROM replies
WHERE workspace_id = @workspace_id
GROUP BY status;

-- name: ListApprovedRepliesByWorkspace :many
SELECT r.id, r.content, r.edited_content,
       m.id AS mention_id, m.platform, m.url, m.title
FROM replies r
JOIN mentions m ON r.mention_id = m.id
WHERE r.workspace_id = @workspace_id AND r.status = 'approved'
ORDER BY r.created_at DESC;

-- name: MarkReplyPosted :one
UPDATE replies
SET status = 'posted', posted_at = NOW(), updated_at = NOW()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;
