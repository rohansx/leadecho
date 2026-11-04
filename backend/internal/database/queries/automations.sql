-- automations (P2 #17)

-- name: ListAutomations :many
SELECT * FROM automations
WHERE workspace_id = @workspace_id
ORDER BY created_at;

-- name: GetAutomation :one
SELECT * FROM automations
WHERE id = @id AND workspace_id = @workspace_id;

-- name: CreateAutomation :one
INSERT INTO automations (workspace_id, name, kind, frequency, run_time, weekday, timezone, enabled, config)
VALUES (@workspace_id, @name, @kind, @frequency, @run_time, @weekday, @timezone, @enabled, @config)
RETURNING *;

-- name: UpdateAutomation :one
UPDATE automations
SET name = @name, kind = @kind, frequency = @frequency, run_time = @run_time,
    weekday = @weekday, timezone = @timezone, enabled = @enabled, config = @config,
    updated_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: DeleteAutomation :execrows
DELETE FROM automations
WHERE id = @id AND workspace_id = @workspace_id;

-- name: ListEnabledAutomations :many
-- Scheduler scan across all workspaces; every follow-up query is scoped by workspace_id.
SELECT * FROM automations
WHERE enabled
ORDER BY workspace_id, created_at;

-- name: SetAutomationLastRun :exec
UPDATE automations SET last_run_at = @last_run_at
WHERE id = @id AND workspace_id = @workspace_id;

-- name: ClaimScheduledAutomationRun :one
-- Returns a row only for the first caller of an (automation, period) slot.
INSERT INTO automation_runs (workspace_id, automation_id, trigger, period_start)
VALUES (@workspace_id, @automation_id, 'schedule', @period_start)
ON CONFLICT (automation_id, period_start) DO NOTHING
RETURNING *;

-- name: CreateManualAutomationRun :one
INSERT INTO automation_runs (workspace_id, automation_id, trigger)
VALUES (@workspace_id, @automation_id, 'manual')
RETURNING *;

-- name: FinishAutomationRun :one
UPDATE automation_runs
SET status = @status, output = @output, error = @error, finished_at = now()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: ListAutomationRuns :many
SELECT * FROM automation_runs
WHERE automation_id = @automation_id AND workspace_id = @workspace_id
ORDER BY started_at DESC
LIMIT @lim;

-- name: AutomationKeywordValue :many
-- Per keyword over mentions created in the last @days days: volume, posted
-- replies and the clicks/signups/revenue of those replies' tracked links.
SELECT
    k.id, k.term,
    COUNT(DISTINCT m.id)::int AS mentions,
    COUNT(DISTINCT r.id) FILTER (WHERE r.status = 'posted')::int AS replies_posted,
    COALESCE(SUM(u.click_count), 0)::int AS clicks,
    COALESCE(SUM(u.signup_count), 0)::int AS signups,
    COALESCE(SUM(u.revenue_cents), 0)::bigint AS revenue_cents
FROM keywords k
LEFT JOIN mentions m ON m.workspace_id = k.workspace_id
    AND (m.keyword_id = k.id OR k.term = ANY(m.keyword_matches))
    AND m.created_at >= NOW() - make_interval(days => @days::int)
LEFT JOIN replies r ON r.mention_id = m.id AND r.workspace_id = k.workspace_id
LEFT JOIN utm_links u ON u.id = r.utm_link_id AND u.workspace_id = k.workspace_id
WHERE k.workspace_id = @workspace_id AND k.is_active
GROUP BY k.id, k.term
ORDER BY revenue_cents DESC, clicks DESC, mentions DESC;
