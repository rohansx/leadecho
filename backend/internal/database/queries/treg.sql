-- name: RecordTregCall :exec
INSERT INTO treg_calls (workspace_id, tool, call_id, cost_micro, status)
VALUES ($1, $2, $3, $4, $5);

-- name: TregSpendByTool :many
SELECT tool,
       COALESCE(SUM(cost_micro), 0)::BIGINT AS cost_micro,
       COUNT(*)::BIGINT AS calls
FROM treg_calls
WHERE workspace_id = $1 AND created_at >= sqlc.arg(since)::timestamptz
GROUP BY tool
ORDER BY cost_micro DESC, tool;
