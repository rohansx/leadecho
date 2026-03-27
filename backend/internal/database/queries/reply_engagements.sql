-- name: CreateReplyEngagement :one
INSERT INTO reply_engagements (
    reply_id, workspace_id, upvotes, downvotes, reply_count, is_removed, checked_at
) VALUES (
    @reply_id, @workspace_id, @upvotes, @downvotes, @reply_count, @is_removed, NOW()
) RETURNING *;

-- name: GetLatestReplyEngagement :one
SELECT * FROM reply_engagements
WHERE reply_id = @reply_id
ORDER BY checked_at DESC
LIMIT 1;

-- name: ListReplyEngagements :many
SELECT * FROM reply_engagements
WHERE reply_id = @reply_id
ORDER BY checked_at DESC
LIMIT @lim;

-- name: ListPostedRepliesSince :many
SELECT r.id, r.mention_id, r.workspace_id, r.content,
       m.url, m.platform
FROM replies r
JOIN mentions m ON r.mention_id = m.id
WHERE r.status = 'posted'
AND r.posted_at >= @since
ORDER BY r.posted_at DESC;
