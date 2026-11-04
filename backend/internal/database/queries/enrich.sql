-- enrich (P1 #11, P2 #16): lead enrichment cap, LinkedIn engagers, tracked profiles.

-- name: CountEnrichedLeadsSince :one
SELECT COUNT(*)::BIGINT FROM leads
WHERE workspace_id = @workspace_id
  AND metadata ? 'enrichment'
  AND (metadata->'enrichment'->>'enriched_at')::timestamptz >= sqlc.arg(since)::timestamptz;

-- name: UpsertEngagerMention :one
-- scoring_metadata.stage keeps these out of the LLM backfill (which would
-- auto-qualify a second lead); relevance is the engager heuristic.
INSERT INTO mentions (
    workspace_id, platform, platform_id, url, content,
    author_username, author_profile_url, status, platform_metadata, keyword_matches,
    relevance_score, conversion_probability, scoring_metadata
) VALUES (
    @workspace_id, 'linkedin', @platform_id, @url, @content,
    @author_username, @author_profile_url, 'new', @platform_metadata, '{}',
    @relevance_score, @conversion_probability, '{"stage":"engager_import","auto_scored":true}'
)
ON CONFLICT (workspace_id, platform, platform_id) DO UPDATE SET
    content = EXCLUDED.content
RETURNING id;

-- name: ListTrackedProfiles :many
SELECT * FROM tracked_profiles
WHERE workspace_id = @workspace_id
ORDER BY created_at DESC;

-- name: CreateTrackedProfile :one
INSERT INTO tracked_profiles (workspace_id, platform, handle, url)
VALUES (@workspace_id, @platform, @handle, @url)
ON CONFLICT (workspace_id, platform, handle) DO UPDATE SET url = EXCLUDED.url
RETURNING *;

-- name: DeleteTrackedProfile :execrows
DELETE FROM tracked_profiles WHERE id = @id AND workspace_id = @workspace_id;

-- name: ListDueTrackedProfiles :many
-- Cross-workspace on purpose (monitor tick); every row carries workspace_id.
SELECT * FROM tracked_profiles
WHERE last_polled_at IS NULL OR last_polled_at < sqlc.arg(before)::timestamptz
ORDER BY last_polled_at NULLS FIRST
LIMIT sqlc.arg(max_rows)::int;

-- name: MarkTrackedProfilePolled :exec
UPDATE tracked_profiles SET last_polled_at = NOW()
WHERE id = @id AND workspace_id = @workspace_id;
