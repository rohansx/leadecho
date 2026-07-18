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
