-- name: GetWorkspaceAccess :one
SELECT paid_data, scanning_paused FROM workspaces WHERE id = @workspace_id;

-- name: GetOpsSetting :one
SELECT value FROM ops_settings WHERE key = @key;

-- name: SetOpsSetting :exec
INSERT INTO ops_settings (key, value, updated_at) VALUES (@key, @value, now())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now();

-- name: OpsListWorkspaces :many
SELECT w.id, w.name, w.created_at, w.plan, w.plan_source, w.credit_balance, w.keyword_limit,
       w.paid_data, w.scanning_paused, w.dodo_subscription_id, w.plan_renews_at,
       coalesce((SELECT u.email FROM users u WHERE u.workspace_id = w.id ORDER BY u.created_at LIMIT 1), '')::text AS owner_email,
       (SELECT count(*) FROM keywords k WHERE k.workspace_id = w.id AND k.is_active)::int AS active_keywords,
       coalesce((SELECT max(m.created_at) FROM mentions m WHERE m.workspace_id = w.id), w.created_at)::timestamptz AS last_post_at,
       coalesce((SELECT sum(c.credits) FROM credit_usage_daily c WHERE c.workspace_id = w.id AND c.day >= CURRENT_DATE - 30), 0)::int AS credits_used_30d,
       coalesce((SELECT sum(t.cost_micro) FROM treg_calls t WHERE t.workspace_id = w.id AND t.created_at >= now() - interval '30 days'), 0)::bigint AS treg_micro_30d,
       coalesce((SELECT sum(e.estimated_cost_usd::numeric) FROM llm_usage_events e WHERE e.workspace_id = w.id AND e.created_at >= now() - interval '30 days'), 0)::float8 AS ai_usd_30d
FROM workspaces w
ORDER BY w.created_at DESC;

-- name: OpsSpendSince :one
SELECT coalesce((SELECT sum(t.cost_micro) FROM treg_calls t WHERE t.created_at >= @since::timestamptz), 0)::bigint AS treg_micro,
       coalesce((SELECT count(*) FROM treg_calls t2 WHERE t2.created_at >= @since::timestamptz), 0)::int AS treg_calls,
       coalesce((SELECT sum(e.estimated_cost_usd::numeric) FROM llm_usage_events e WHERE e.created_at >= @since::timestamptz), 0)::float8 AS ai_usd;

-- name: OpsSetPlan :exec
UPDATE workspaces SET plan = @plan, plan_source = @plan_source, keyword_limit = @keyword_limit, updated_at = now()
WHERE id = @workspace_id;

-- name: OpsSetPaidData :exec
UPDATE workspaces SET paid_data = @paid_data, updated_at = now() WHERE id = @workspace_id;

-- name: OpsSetScanningPaused :exec
UPDATE workspaces SET scanning_paused = @scanning_paused, updated_at = now() WHERE id = @workspace_id;

-- name: InsertOpsAudit :exec
INSERT INTO ops_audit (actor_email, action, workspace_id, detail)
VALUES (@actor_email, @action, sqlc.narg(workspace_id), @detail);

-- name: ListOpsAudit :many
SELECT a.actor_email, a.action, a.workspace_id, coalesce(w.name, '')::text AS workspace_name, a.detail, a.created_at
FROM ops_audit a LEFT JOIN workspaces w ON w.id = a.workspace_id
ORDER BY a.created_at DESC LIMIT @lim;
