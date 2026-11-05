-- ─── Brand profile (onboarding) ───────────────────────

-- name: GetBrandProfile :one
SELECT * FROM brand_profiles WHERE workspace_id = @workspace_id;

-- name: UpsertBrandProfile :one
INSERT INTO brand_profiles (
    workspace_id, overview, product, positioning, customer_segments,
    competitors, pain_points, keywords, features, target_audience, source_url
) VALUES (
    @workspace_id, @overview, @product, @positioning, @customer_segments,
    @competitors, @pain_points, @keywords, @features, @target_audience, @source_url
)
ON CONFLICT (workspace_id) DO UPDATE SET
    overview = EXCLUDED.overview,
    product = EXCLUDED.product,
    positioning = EXCLUDED.positioning,
    customer_segments = EXCLUDED.customer_segments,
    competitors = EXCLUDED.competitors,
    pain_points = EXCLUDED.pain_points,
    keywords = EXCLUDED.keywords,
    features = EXCLUDED.features,
    target_audience = EXCLUDED.target_audience,
    source_url = EXCLUDED.source_url
RETURNING *;

-- ─── Listener builder ─────────────────────────────────

-- name: SetMonitoringProfileIntent :exec
UPDATE monitoring_profiles
SET looks_for = @looks_for, skips = @skips, goal = @goal
WHERE id = @id AND workspace_id = @workspace_id;

-- name: UpsertSampleFeedback :one
INSERT INTO onboarding_sample_feedback (workspace_id, goal, platform, url, text, label)
VALUES (@workspace_id, @goal, @platform, @url, @text, @label)
ON CONFLICT (workspace_id, goal, url) DO UPDATE SET
    label = EXCLUDED.label,
    text = EXCLUDED.text,
    platform = EXCLUDED.platform
RETURNING *;

-- name: LinkSampleFeedbackToProfile :exec
UPDATE onboarding_sample_feedback
SET profile_id = @profile_id
WHERE workspace_id = @workspace_id AND goal = @goal AND profile_id IS NULL;

-- name: ListSampleFeedbackForProfile :many
SELECT * FROM onboarding_sample_feedback
WHERE workspace_id = @workspace_id AND profile_id = @profile_id
ORDER BY created_at DESC
LIMIT 50;

-- ─── KB seed ──────────────────────────────────────────

-- name: CountDocumentsBySourceURL :one
SELECT COUNT(*)::int FROM documents
WHERE workspace_id = @workspace_id AND source_url = @source_url;
