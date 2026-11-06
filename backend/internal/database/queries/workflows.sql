-- name: ListActiveWorkflowsByWorkspace :many
SELECT * FROM workflows
WHERE workspace_id = @workspace_id AND status = 'active'
ORDER BY created_at ASC;

-- name: HasWorkflowExecutionForMention :one
SELECT EXISTS(
    SELECT 1 FROM workflow_executions
    WHERE workflow_id = @workflow_id AND mention_id = @mention_id
) AS exists;

-- name: CreateWorkflowExecution :one
INSERT INTO workflow_executions (
    workflow_id, mention_id, status, steps, current_step
) VALUES (
    @workflow_id, @mention_id, @status, @steps, @current_step
) RETURNING *;

-- name: IncrementWorkflowTriggerCount :one
UPDATE workflows
SET execution_count = execution_count + 1,
    last_triggered_at = NOW(),
    updated_at = NOW()
WHERE id = @id
RETURNING *;

-- name: UpdateWorkflowExecution :one
UPDATE workflow_executions
SET status = @status,
    current_step = @current_step,
    steps = @steps,
    error_message = @error_message,
    completed_at = @completed_at
WHERE id = @id
RETURNING *;

-- name: ListWorkflows :many
SELECT * FROM workflows
WHERE workspace_id = @workspace_id
ORDER BY created_at ASC;

-- name: GetWorkflow :one
SELECT * FROM workflows
WHERE id = @id AND workspace_id = @workspace_id;

-- name: CountWorkflows :one
SELECT count(*) FROM workflows WHERE workspace_id = @workspace_id;

-- name: CreateWorkflow :one
INSERT INTO workflows (workspace_id, name, description, status, trigger_config, action_chain, max_triggers_per_hour, created_by)
VALUES (@workspace_id, @name, @description, @status, @trigger_config, @action_chain, @max_triggers_per_hour, @created_by)
RETURNING *;

-- name: UpdateWorkflow :one
UPDATE workflows
SET name = @name, description = @description, status = @status,
    trigger_config = @trigger_config, action_chain = @action_chain,
    max_triggers_per_hour = @max_triggers_per_hour, updated_at = NOW()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: DeleteWorkflow :execrows
DELETE FROM workflows WHERE id = @id AND workspace_id = @workspace_id;

-- name: CountWorkflowExecutionsSince :one
SELECT count(*) FROM workflow_executions
WHERE workflow_id = @workflow_id AND created_at > @since;

-- name: ListWorkflowExecutions :many
SELECT e.id, e.status, e.steps, e.error_message, e.created_at,
       m.id AS mention_id, m.platform, m.title, m.url, m.relevance_score
FROM workflow_executions e
JOIN workflows w ON w.id = e.workflow_id
LEFT JOIN mentions m ON m.id = e.mention_id
WHERE e.workflow_id = @workflow_id AND w.workspace_id = @workspace_id
ORDER BY e.created_at DESC
LIMIT 50;

-- name: ListRecentScoredMentions :many
-- Preview corpus for "test this workflow": the latest classified posts.
SELECT id, platform, title, content, url, author_username, relevance_score, intent_tags
FROM mentions
WHERE workspace_id = @workspace_id AND relevance_score IS NOT NULL
ORDER BY created_at DESC
LIMIT @lim;
