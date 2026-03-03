-- name: ListNotifications :many
SELECT * FROM notifications
WHERE workspace_id = @workspace_id
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: CreateNotification :one
INSERT INTO notifications (
    workspace_id, channel, recipient, subject, body, metadata, sent_at
) VALUES (
    @workspace_id, @channel, @recipient, @subject, @body, @metadata, @sent_at
) RETURNING *;

-- name: CountNotifications :one
SELECT COUNT(*)::int as count FROM notifications
WHERE workspace_id = @workspace_id;
