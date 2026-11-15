-- competitor watch: competitors, their sources, fetched items, moves.

-- name: ListCompetitors :many
SELECT * FROM competitors WHERE workspace_id = @workspace_id ORDER BY created_at, name;

-- name: GetCompetitor :one
SELECT * FROM competitors WHERE id = @id AND workspace_id = @workspace_id;

-- name: GetCompetitorByID :one
-- Poll loop only: the id comes from a row that already carries workspace_id.
SELECT * FROM competitors WHERE id = @id;

-- name: CountCompetitors :one
SELECT COUNT(*)::INT FROM competitors WHERE workspace_id = @workspace_id;

-- name: CreateCompetitor :one
INSERT INTO competitors (workspace_id, name, website)
VALUES (@workspace_id, @name, @website)
RETURNING *;

-- name: GetCompetitorByName :one
SELECT * FROM competitors WHERE workspace_id = @workspace_id AND lower(name) = lower(@name);

-- name: UpdateCompetitor :one
UPDATE competitors SET website = @website, status = @status
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: DeleteCompetitor :execrows
DELETE FROM competitors WHERE id = @id AND workspace_id = @workspace_id;

-- name: SetCompetitorSummary :exec
UPDATE competitors SET summary = @summary, summary_at = NOW() WHERE id = @id;

-- name: ListCompetitorSources :many
SELECT * FROM competitor_sources WHERE workspace_id = @workspace_id ORDER BY kind, url;

-- name: CountCompetitorSources :one
SELECT COUNT(*)::INT FROM competitor_sources WHERE competitor_id = @competitor_id;

-- name: CreateCompetitorSource :execrows
INSERT INTO competitor_sources (competitor_id, workspace_id, kind, url)
VALUES (@competitor_id, @workspace_id, @kind, @url)
ON CONFLICT (competitor_id, kind, url) DO NOTHING;

-- name: DeleteCompetitorSource :execrows
DELETE FROM competitor_sources
WHERE id = @id AND competitor_id = @competitor_id AND workspace_id = @workspace_id;

-- name: DeleteCompetitorSources :exec
DELETE FROM competitor_sources WHERE competitor_id = @competitor_id AND workspace_id = @workspace_id;

-- name: ListDueCompetitorSources :many
-- Cross-workspace on purpose (monitor tick); every row carries workspace_id.
SELECT s.* FROM competitor_sources s
JOIN competitors c ON c.id = s.competitor_id
WHERE NOT s.broken AND c.status = 'active'
  AND (s.last_checked_at IS NULL OR s.last_checked_at < sqlc.arg(before)::timestamptz)
ORDER BY s.last_checked_at NULLS FIRST
LIMIT sqlc.arg(max_rows)::int;

-- name: MarkCompetitorSourceChecked :exec
UPDATE competitor_sources
SET last_checked_at = NOW(), failures = 0, last_hash = @last_hash, last_text = @last_text
WHERE id = @id;

-- name: TouchCompetitorSource :exec
-- Checked without a result (paid data locked): wait a day, count no failure.
UPDATE competitor_sources SET last_checked_at = NOW() WHERE id = @id;

-- name: MarkCompetitorSourceFailed :exec
UPDATE competitor_sources
SET last_checked_at = NOW(), failures = failures + 1, broken = (failures + 1 >= 5)
WHERE id = @id;

-- name: InsertCompetitorItem :execrows
INSERT INTO competitor_items (source_id, competitor_id, workspace_id, item_key, title, body, url, happened_at, processed_at)
VALUES (@source_id, @competitor_id, @workspace_id, @item_key, @title, @body, @url, @happened_at, sqlc.narg(processed_at))
ON CONFLICT (source_id, item_key) DO NOTHING;

-- name: ListUnprocessedCompetitorItems :many
SELECT * FROM competitor_items
WHERE processed_at IS NULL
ORDER BY created_at
LIMIT sqlc.arg(max_rows)::int;

-- name: MarkCompetitorItemsProcessed :exec
UPDATE competitor_items SET processed_at = NOW() WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: BumpCompetitorItemAttempts :exec
-- A failed read: count it, and stop retrying at max_attempts.
UPDATE competitor_items
SET attempts = attempts + 1,
    processed_at = CASE WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN NOW() ELSE processed_at END
WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: CreateCompetitorMove :one
INSERT INTO competitor_moves (competitor_id, workspace_id, kind, title, summary, urls, happened_at, impact, action, theme_id, importance)
VALUES (@competitor_id, @workspace_id, @kind, @title, @summary, @urls, @happened_at, @impact, @action, @theme_id, @importance)
RETURNING *;

-- name: AppendCompetitorMoveURL :exec
UPDATE competitor_moves SET urls = array_append(urls, sqlc.arg(url)::text)
WHERE id = @id AND competitor_id = @competitor_id AND NOT (sqlc.arg(url)::text = ANY(urls));

-- name: ListCompetitorMoves :many
SELECT * FROM competitor_moves
WHERE workspace_id = @workspace_id AND happened_at >= sqlc.arg(since)::timestamptz
ORDER BY happened_at DESC;

-- name: ListMovesForCompetitor :many
SELECT * FROM competitor_moves
WHERE competitor_id = @competitor_id AND happened_at >= sqlc.arg(since)::timestamptz
ORDER BY happened_at DESC;

-- name: ListCompetitorMovesBetween :many
SELECT m.*, c.name AS competitor_name FROM competitor_moves m
JOIN competitors c ON c.id = m.competitor_id
WHERE m.workspace_id = @workspace_id
  AND m.created_at >= sqlc.arg(from_at)::timestamptz AND m.created_at < sqlc.arg(to_at)::timestamptz
ORDER BY (m.importance = 'high') DESC, m.happened_at DESC;

-- name: ListUnalertedHighCompetitorMoves :many
-- Cross-workspace on purpose (monitor tick).
SELECT m.*, c.name AS competitor_name FROM competitor_moves m
JOIN competitors c ON c.id = m.competitor_id
WHERE m.importance = 'high' AND m.alerted_at IS NULL
ORDER BY m.created_at
LIMIT sqlc.arg(max_rows)::int;

-- name: MarkCompetitorMoveAlerted :exec
UPDATE competitor_moves SET alerted_at = NOW() WHERE id = @id;

-- name: ListWorkspacesNeedingCompetitorSetup :many
-- Workspaces with competitor names in their brand profile and no setup yet.
SELECT b.workspace_id FROM brand_profiles b
LEFT JOIN competitor_setup s ON s.workspace_id = b.workspace_id
WHERE s.workspace_id IS NULL AND cardinality(b.competitors) > 0
ORDER BY random() -- a workspace whose setup keeps failing must not block the rest
LIMIT sqlc.arg(max_rows)::int;

-- name: GetCompetitorSetup :one
SELECT * FROM competitor_setup WHERE workspace_id = @workspace_id;

-- name: MarkCompetitorSetup :exec
INSERT INTO competitor_setup (workspace_id, unresolved) VALUES (@workspace_id, @unresolved)
ON CONFLICT (workspace_id) DO UPDATE SET unresolved = EXCLUDED.unresolved, setup_at = NOW();

-- name: GetWorkspacePlan :one
SELECT plan FROM workspaces WHERE id = @id;

-- name: ListCompetitorPeople :many
-- People complaining about, or looking to leave, a named competitor.
-- name_pattern is a case-insensitive regular expression built by the caller.
-- "Recent" goes by when the post was written, not when LeadEcho found it: a
-- years-old thread found last week is not someone unhappy today.
SELECT id, platform, url, content, author_username, relevance_score, intent_tags,
       COALESCE(platform_created_at, created_at)::timestamptz AS posted_at
FROM mentions
WHERE workspace_id = @workspace_id
  AND intent_tags && ARRAY['competitor_complaint', 'seeking_alternative']::text[]
  AND COALESCE(platform_created_at, created_at) >= sqlc.arg(since)::timestamptz
  AND content ~* sqlc.arg(name_pattern)::text
ORDER BY COALESCE(platform_created_at, created_at) DESC
LIMIT sqlc.arg(max_rows)::int;

-- name: CountCompetitorPeople :one
SELECT COUNT(*)::INT FROM mentions
WHERE workspace_id = @workspace_id
  AND intent_tags && ARRAY['competitor_complaint', 'seeking_alternative']::text[]
  AND COALESCE(platform_created_at, created_at) >= sqlc.arg(since)::timestamptz
  AND content ~* sqlc.arg(name_pattern)::text;
