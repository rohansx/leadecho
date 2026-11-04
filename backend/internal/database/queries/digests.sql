-- Digests (P1 #13)

-- name: GetDigestSettings :one
SELECT * FROM digest_settings WHERE workspace_id = @workspace_id;

-- name: UpsertDigestSettings :one
INSERT INTO digest_settings (workspace_id, daily_enabled, daily_time, weekly_enabled, weekly_day, timezone, recipients, channels)
VALUES (@workspace_id, @daily_enabled, @daily_time, @weekly_enabled, @weekly_day, @timezone, @recipients::text[], @channels::text[])
ON CONFLICT (workspace_id) DO UPDATE SET
    daily_enabled = EXCLUDED.daily_enabled,
    daily_time = EXCLUDED.daily_time,
    weekly_enabled = EXCLUDED.weekly_enabled,
    weekly_day = EXCLUDED.weekly_day,
    timezone = EXCLUDED.timezone,
    recipients = EXCLUDED.recipients,
    channels = EXCLUDED.channels,
    updated_at = now()
RETURNING *;

-- name: ListDigestSchedules :many
-- Every workspace with its digest schedule; workspaces without a row get the defaults.
SELECT w.id AS workspace_id,
       COALESCE(s.daily_enabled, true)::bool AS daily_enabled,
       COALESCE(s.daily_time, '09:30')::text AS daily_time,
       COALESCE(s.weekly_enabled, true)::bool AS weekly_enabled,
       COALESCE(s.weekly_day, 'monday')::text AS weekly_day,
       COALESCE(s.timezone, 'UTC')::text AS timezone,
       COALESCE(s.recipients, '{}')::text[] AS recipients,
       COALESCE(s.channels, '{email}')::text[] AS channels
FROM workspaces w
LEFT JOIN digest_settings s ON s.workspace_id = w.id;

-- name: GetWorkspaceOwnerEmail :one
SELECT email FROM users
WHERE workspace_id = @workspace_id AND is_active = true
ORDER BY (role = 'admin') DESC, created_at
LIMIT 1;

-- name: ClaimDigestRun :one
-- Returns a row only when this caller owns the (workspace, kind, period) slot:
-- a fresh insert, a retry of a failed run (max 5 attempts, backoff of
-- 10 min x attempts so a provider outage is ridden out inside the due window),
-- or a run left 'pending' >15 min by a sender that crashed mid-send.
-- Also refuses a new slot within min_gap_seconds of a run already sent or
-- sending: editing the send time must not produce a second digest that day.
INSERT INTO digest_runs (workspace_id, kind, period_start, period_end)
SELECT @workspace_id::uuid, @kind::text, @period_start::timestamptz, @period_end::timestamptz
WHERE NOT EXISTS (
    SELECT 1 FROM digest_runs o
    WHERE o.workspace_id = @workspace_id::uuid AND o.kind = @kind::text
      AND o.period_start <> @period_start::timestamptz
      AND o.status IN ('pending', 'sent')
      AND abs(extract(epoch FROM (o.period_end - @period_end::timestamptz))) < @min_gap_seconds::float8
)
ON CONFLICT (workspace_id, kind, period_start) DO UPDATE
    SET status = 'pending', attempts = digest_runs.attempts + 1, updated_at = now()
    WHERE (digest_runs.status = 'failed' AND digest_runs.attempts < 5
           AND digest_runs.updated_at < now() - make_interval(mins => 10 * digest_runs.attempts))
       OR (digest_runs.status = 'pending' AND digest_runs.attempts < 5
           AND digest_runs.updated_at < now() - interval '15 minutes')
RETURNING id;

-- name: FinishDigestRun :exec
UPDATE digest_runs SET status = @status, error = @error, updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id;

-- name: DigestTopMentions :many
SELECT m.id, m.platform, m.url, m.title, m.content, m.author_username,
       m.relevance_score, m.intent, m.intent_tags
FROM mentions m
WHERE m.workspace_id = @workspace_id
  AND m.created_at >= @period_start AND m.created_at < @period_end
  AND m.relevance_score IS NOT NULL
  AND m.status NOT IN ('spam', 'archived')
ORDER BY m.relevance_score DESC, m.created_at DESC
LIMIT @max_rows;

-- name: DigestPlatformCounts :many
SELECT platform, COUNT(*)::int AS count
FROM mentions
WHERE workspace_id = @workspace_id
  AND created_at >= @period_start AND created_at < @period_end
GROUP BY platform
ORDER BY count DESC;

-- name: DigestIntentCounts :many
SELECT intent::text AS intent, COUNT(*)::int AS count
FROM mentions
WHERE workspace_id = @workspace_id
  AND intent IS NOT NULL
  AND created_at >= @period_start AND created_at < @period_end
GROUP BY intent
ORDER BY count DESC;

-- name: DigestKeywordStats :many
SELECT k.term,
       COUNT(m.id)::int AS mention_count,
       COALESCE(AVG(m.relevance_score), 0)::float8 AS avg_score
FROM keywords k
JOIN mentions m ON (m.keyword_id = k.id OR k.term = ANY(m.keyword_matches))
    AND m.workspace_id = k.workspace_id
    AND m.created_at >= @period_start AND m.created_at < @period_end
WHERE k.workspace_id = @workspace_id
GROUP BY k.id, k.term
ORDER BY mention_count DESC
LIMIT 50;

-- name: DigestRepliesPosted :one
SELECT COUNT(*)::int AS count FROM replies
WHERE workspace_id = @workspace_id AND status = 'posted'
  AND COALESCE(posted_at, updated_at) >= @period_start::timestamptz
  AND COALESCE(posted_at, updated_at) < @period_end::timestamptz;

-- name: DigestAttribution :one
SELECT
    COUNT(*) FILTER (WHERE e.event_type = 'click')::int AS clicks,
    COUNT(*) FILTER (WHERE e.event_type = 'signup')::int AS signups,
    COALESCE(SUM(e.revenue_cents) FILTER (WHERE e.event_type IN ('signup', 'purchase')), 0)::bigint AS revenue_cents
FROM utm_events e
JOIN utm_links u ON u.id = e.utm_link_id
WHERE u.workspace_id = @workspace_id
  AND e.created_at >= @period_start AND e.created_at < @period_end;
