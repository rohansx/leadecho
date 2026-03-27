-- name: CreateOnboardingAnalysis :one
INSERT INTO onboarding_analyses (
    workspace_id, source_url, raw_text, analysis, status
) VALUES (
    @workspace_id, @source_url, @raw_text, @analysis, @status
) RETURNING *;

-- name: GetLatestOnboardingAnalysis :one
SELECT * FROM onboarding_analyses
WHERE workspace_id = @workspace_id
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateOnboardingAnalysis :one
UPDATE onboarding_analyses
SET analysis = @analysis, status = @status, error_message = @error_message
WHERE id = @id
RETURNING *;
