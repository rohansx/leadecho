-- chat (P2 #15). Every query is scoped by workspace_id and, for threads, user_id.

-- name: CreateChatThread :one
INSERT INTO chat_threads (workspace_id, user_id, title)
VALUES (@workspace_id, @user_id, @title)
RETURNING *;

-- name: GetChatThread :one
SELECT * FROM chat_threads
WHERE id = @id AND workspace_id = @workspace_id AND user_id = @user_id;

-- name: ListChatThreads :many
SELECT * FROM chat_threads
WHERE workspace_id = @workspace_id AND user_id = @user_id
ORDER BY updated_at DESC
LIMIT 50;

-- name: TouchChatThread :exec
UPDATE chat_threads SET updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id;

-- name: DeleteChatThread :execrows
DELETE FROM chat_threads
WHERE id = @id AND workspace_id = @workspace_id AND user_id = @user_id;

-- name: CreateChatMessage :one
INSERT INTO chat_messages (thread_id, workspace_id, role, content, tool_calls)
VALUES (@thread_id, @workspace_id, @role, @content, @tool_calls)
RETURNING *;

-- name: ListChatMessages :many
SELECT * FROM (
    SELECT * FROM chat_messages
    WHERE thread_id = @thread_id AND workspace_id = @workspace_id
    ORDER BY created_at DESC
    LIMIT @lim
) recent ORDER BY created_at ASC;

-- name: GetChatMessageForUser :one
SELECT m.* FROM chat_messages m
JOIN chat_threads t ON t.id = m.thread_id
WHERE m.id = @id AND m.workspace_id = @workspace_id AND t.user_id = @user_id;

-- name: SwapChatMessageToolCalls :execrows
-- Compare-and-swap so a confirm runs a write tool at most once.
UPDATE chat_messages SET tool_calls = @new_calls
WHERE id = @id AND workspace_id = @workspace_id AND tool_calls = @old_calls;
