-- name: GetThreadByMention :one
SELECT * FROM threads
WHERE mention_id = @mention_id
LIMIT 1;

-- name: CreateThread :one
INSERT INTO threads (
    mention_id, platform, thread_id, content, fetched_at
) VALUES (
    @mention_id, @platform, @thread_id, @content, NOW()
) RETURNING *;

-- name: UpdateThreadContent :one
UPDATE threads
SET content = @content, fetched_at = NOW()
WHERE id = @id
RETURNING *;
