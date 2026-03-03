-- name: ListKeywords :many
SELECT id, workspace_id, term,
    platforms::text[] as platforms,
    is_active, match_type, negative_terms, subreddits, created_at, updated_at
FROM keywords
WHERE workspace_id = @workspace_id
ORDER BY created_at DESC;

-- name: ListActiveKeywords :many
SELECT id, workspace_id, term,
    platforms::text[] as platforms,
    is_active, match_type, negative_terms, subreddits, created_at, updated_at
FROM keywords
WHERE workspace_id = @workspace_id AND is_active = true
ORDER BY created_at DESC;

-- name: GetKeyword :one
SELECT id, workspace_id, term,
    platforms::text[] as platforms,
    is_active, match_type, negative_terms, subreddits, created_at, updated_at
FROM keywords
WHERE id = @id AND workspace_id = @workspace_id;

-- name: CreateKeyword :one
INSERT INTO keywords (
    workspace_id, term, platforms, is_active, match_type, negative_terms, subreddits
) VALUES (
    @workspace_id, @term, @platforms::platform_type[], @is_active, @match_type, @negative_terms, @subreddits
) RETURNING id, workspace_id, term,
    platforms::text[] as platforms,
    is_active, match_type, negative_terms, subreddits, created_at, updated_at;

-- name: UpdateKeyword :one
UPDATE keywords
SET term = @term,
    platforms = @platforms::platform_type[],
    is_active = @is_active,
    match_type = @match_type,
    negative_terms = @negative_terms,
    subreddits = @subreddits
WHERE id = @id AND workspace_id = @workspace_id
RETURNING id, workspace_id, term,
    platforms::text[] as platforms,
    is_active, match_type, negative_terms, subreddits, created_at, updated_at;

-- name: DeleteKeyword :exec
DELETE FROM keywords
WHERE id = @id AND workspace_id = @workspace_id;

-- name: ListAllActiveKeywords :many
SELECT id, workspace_id, term,
    platforms::text[] as platforms,
    is_active, match_type, negative_terms, subreddits, created_at, updated_at
FROM keywords
WHERE is_active = true
ORDER BY workspace_id, created_at DESC;

-- name: CountKeywords :one
SELECT COUNT(*)::int as count FROM keywords
WHERE workspace_id = @workspace_id;
