-- name: ListLeads :many
SELECT * FROM leads
WHERE workspace_id = @workspace_id
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: ListLeadsByStage :many
SELECT * FROM leads
WHERE workspace_id = @workspace_id AND stage = @stage
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: GetLead :one
SELECT * FROM leads
WHERE id = @id AND workspace_id = @workspace_id;

-- name: GetLeadByMention :one
SELECT * FROM leads
WHERE mention_id = @mention_id AND workspace_id = @workspace_id
ORDER BY created_at DESC
LIMIT 1;

-- name: CreateLead :one
INSERT INTO leads (
    workspace_id, mention_id, stage,
    contact_name, contact_email, company,
    username, platform, profile_url,
    estimated_value, notes, tags, metadata
) VALUES (
    @workspace_id, @mention_id, @stage,
    @contact_name, @contact_email, @company,
    @username, @platform, @profile_url,
    @estimated_value, @notes, @tags, @metadata
) RETURNING *;

-- name: UpdateLeadStage :one
UPDATE leads
SET stage = @stage
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: UpdateLead :one
UPDATE leads
SET
    stage = COALESCE(@stage, stage),
    contact_name = COALESCE(@contact_name, contact_name),
    contact_email = COALESCE(@contact_email, contact_email),
    company = COALESCE(@company, company),
    notes = COALESCE(@notes, notes),
    estimated_value = COALESCE(@estimated_value, estimated_value)
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: ListLeadsWithMention :many
-- Pipeline list: each lead plus what the person actually wrote, best first.
SELECT sqlc.embed(l),
       m.title               AS mention_title,
       m.content             AS mention_content,
       m.url                 AS mention_url,
       m.intent              AS mention_intent,
       m.relevance_score     AS mention_score,
       m.platform_created_at AS mention_posted_at
FROM leads l
LEFT JOIN mentions m ON m.id = l.mention_id AND m.workspace_id = l.workspace_id
WHERE l.workspace_id = @workspace_id
  AND (sqlc.narg('stage')::lead_stage IS NULL OR l.stage = sqlc.narg('stage')::lead_stage)
ORDER BY m.relevance_score DESC NULLS LAST, l.created_at DESC
LIMIT @lim OFFSET @off;

-- name: CountLeadsByStage :many
SELECT stage, COUNT(*)::int as count
FROM leads
WHERE workspace_id = @workspace_id
GROUP BY stage;

-- name: CreateLeadEvent :one
INSERT INTO lead_events (
    lead_id, previous_stage, new_stage, changed_by, notes
) VALUES (
    @lead_id, @previous_stage, @new_stage, @changed_by, @notes
) RETURNING *;

-- name: ListLeadEvents :many
SELECT * FROM lead_events
WHERE lead_id = @lead_id
ORDER BY created_at DESC;
