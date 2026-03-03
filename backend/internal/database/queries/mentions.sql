-- name: ListMentions :many
SELECT * FROM mentions
WHERE workspace_id = @workspace_id
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: ListMentionsByStatus :many
SELECT * FROM mentions
WHERE workspace_id = @workspace_id AND status = @status
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: ListMentionsByPlatform :many
SELECT * FROM mentions
WHERE workspace_id = @workspace_id AND platform = @platform
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: ListMentionsByIntent :many
SELECT * FROM mentions
WHERE workspace_id = @workspace_id AND intent = @intent
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: GetMention :one
SELECT * FROM mentions
WHERE id = @id AND workspace_id = @workspace_id;

-- name: CreateMention :one
INSERT INTO mentions (
    workspace_id, keyword_id, platform, platform_id, url,
    title, content, author_username, author_profile_url,
    author_karma, author_account_age_days,
    relevance_score, intent, conversion_probability, status,
    platform_metadata, engagement_metrics, keyword_matches,
    platform_created_at
) VALUES (
    @workspace_id, @keyword_id, @platform, @platform_id, @url,
    @title, @content, @author_username, @author_profile_url,
    @author_karma, @author_account_age_days,
    @relevance_score, @intent, @conversion_probability, @status,
    @platform_metadata, @engagement_metrics, @keyword_matches,
    @platform_created_at
) RETURNING *;

-- name: UpdateMentionStatus :one
UPDATE mentions
SET status = @status
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: AssignMention :one
UPDATE mentions
SET assigned_to = @assigned_to
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: CountMentionsByStatus :many
SELECT status, COUNT(*)::int as count
FROM mentions
WHERE workspace_id = @workspace_id
GROUP BY status;

-- name: CountMentionsByPlatform :many
SELECT platform, COUNT(*)::int as count
FROM mentions
WHERE workspace_id = @workspace_id
GROUP BY platform;

-- name: SearchMentions :many
SELECT * FROM mentions
WHERE workspace_id = @workspace_id
AND content_tsv @@ plainto_tsquery('english', @query)
ORDER BY ts_rank(content_tsv, plainto_tsquery('english', @query)) DESC
LIMIT @lim OFFSET @off;

-- name: UpdateMentionIntent :one
UPDATE mentions
SET intent = @intent,
    conversion_probability = @conversion_probability,
    relevance_score = @relevance_score
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: ListUnclassifiedMentions :many
SELECT * FROM mentions
WHERE workspace_id = @workspace_id AND intent IS NULL
ORDER BY created_at DESC
LIMIT @lim;

-- ─── Embedding & Scoring ──────────────────────────────

-- name: UpdateMentionEmbedding :exec
UPDATE mentions
SET content_embedding = @content_embedding
WHERE id = @id;

-- name: UpdateMentionScoring :one
UPDATE mentions
SET intent = @intent,
    conversion_probability = @conversion_probability,
    relevance_score = @relevance_score,
    scoring_metadata = @scoring_metadata
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- ─── Smart Inbox Tiers ────────────────────────────────

-- name: ListMentionsLeadsReady :many
SELECT * FROM mentions
WHERE workspace_id = @workspace_id
AND relevance_score >= 7.0
AND intent IN ('buy_signal', 'recommendation_ask', 'complaint')
ORDER BY relevance_score DESC, created_at DESC
LIMIT @lim OFFSET @off;

-- name: ListMentionsWorthWatching :many
SELECT * FROM mentions
WHERE workspace_id = @workspace_id
AND relevance_score IS NOT NULL
AND relevance_score >= 4.0
AND relevance_score < 7.0
ORDER BY relevance_score DESC, created_at DESC
LIMIT @lim OFFSET @off;

-- name: ListMentionsFiltered :many
SELECT * FROM mentions
WHERE workspace_id = @workspace_id
AND (relevance_score IS NULL OR relevance_score < 4.0)
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: ListRecentLeadsForWorkspace :many
SELECT id, platform, url, title, content, author_username,
       intent, relevance_score, created_at
FROM mentions
WHERE workspace_id = @workspace_id
  AND (
    (relevance_score >= 7.0 AND intent IN ('buy_signal', 'recommendation_ask', 'complaint'))
    OR (relevance_score >= 4.0 AND relevance_score < 7.0)
  )
ORDER BY created_at DESC
LIMIT @lim;

-- name: CountMentionsByTier :many
SELECT
    CASE
        WHEN relevance_score >= 7.0 AND intent IN ('buy_signal', 'recommendation_ask', 'complaint') THEN 'leads_ready'
        WHEN relevance_score >= 4.0 AND relevance_score < 7.0 THEN 'worth_watching'
        ELSE 'filtered'
    END as tier,
    COUNT(*)::int as count
FROM mentions
WHERE workspace_id = @workspace_id
GROUP BY tier;
