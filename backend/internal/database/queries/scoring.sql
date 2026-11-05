-- scoring: dedupe, intent tags, human feedback, listener intent.

-- name: FindRecentDuplicateMention :one
-- Same canonical URL, or a simhash within max_distance bits (-1 disables the
-- text check), in the same workspace within the lookback window.
SELECT EXISTS (
    SELECT 1 FROM mentions
    WHERE workspace_id = @workspace_id
      AND created_at > now() - make_interval(days => @lookback_days::int)
      AND (
        canonical_url = @canonical_url::text
        OR (@max_distance::int >= 0 AND content_simhash IS NOT NULL
            AND bit_count((content_simhash # @simhash::bigint)::bit(64)) <= @max_distance::int)
      )
)::bool AS duplicate;

-- name: SetMentionDedupeKeys :exec
UPDATE mentions
SET canonical_url = @canonical_url, content_simhash = @content_simhash
WHERE id = @id AND workspace_id = @workspace_id;

-- name: UpdateMentionTags :exec
UPDATE mentions
SET intent_tags = @intent_tags::text[], sentiment = @sentiment
WHERE id = @id AND workspace_id = @workspace_id;

-- name: GetMentionScoringProfile :one
-- The listener (monitoring profile) a mention came from, via its keyword.
SELECT mp.id, mp.name, mp.description, mp.looks_for, mp.skips
FROM mentions m
JOIN keywords k ON k.id = m.keyword_id AND k.workspace_id = m.workspace_id
JOIN monitoring_profiles mp ON mp.id = k.profile_id AND mp.workspace_id = m.workspace_id
WHERE m.id = @mention_id AND m.workspace_id = @workspace_id;

-- name: UpsertMentionFeedback :one
INSERT INTO mention_feedback (workspace_id, mention_id, profile_id, user_id, label, note)
VALUES (@workspace_id, @mention_id, @profile_id, @user_id, @label, @note)
ON CONFLICT (mention_id, user_id) DO UPDATE
SET label = EXCLUDED.label, note = EXCLUDED.note, profile_id = EXCLUDED.profile_id, updated_at = now()
RETURNING *;

-- name: ListScoringExamples :many
-- Latest labelled mentions for one listener; profile_id NULL = mentions with
-- no listener (workspace-level examples).
SELECT f.label, f.note, COALESCE(m.title, '')::text AS title, m.content
FROM mention_feedback f
JOIN mentions m ON m.id = f.mention_id AND m.workspace_id = f.workspace_id
WHERE f.workspace_id = @workspace_id
  AND f.profile_id IS NOT DISTINCT FROM sqlc.narg(profile_id)::uuid
ORDER BY f.updated_at DESC
LIMIT @lim;

-- name: UpdateMonitoringProfileIntent :one
UPDATE monitoring_profiles
SET looks_for = @looks_for, skips = @skips
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;
