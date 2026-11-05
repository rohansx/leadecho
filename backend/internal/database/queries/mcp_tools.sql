-- Queries backing the agent tool layer (internal/tools), shared by the MCP
-- server and the in-app AI chat.

-- name: ToolsUpdateListenerIntent :one
UPDATE monitoring_profiles
SET looks_for = @looks_for, skips = @skips, updated_at = NOW()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: ToolsAttributionByPlatform :many
-- Revenue and funnel per source platform for replies created in the window.
SELECT
    m.platform,
    COUNT(r.id) FILTER (WHERE r.status = 'posted')::int AS replies_posted,
    COALESCE(SUM(u.click_count), 0)::int AS clicks,
    COALESCE(SUM(u.signup_count), 0)::int AS signups,
    COALESCE(SUM(u.revenue_cents), 0)::bigint AS revenue_cents
FROM replies r
JOIN mentions m ON m.id = r.mention_id AND m.workspace_id = r.workspace_id
LEFT JOIN utm_links u ON u.id = r.utm_link_id AND u.workspace_id = r.workspace_id
WHERE r.workspace_id = @workspace_id
  AND r.created_at >= NOW() - make_interval(days => @days::int)
GROUP BY m.platform
ORDER BY revenue_cents DESC, signups DESC;

-- name: ToolsWorkspaceSummary :one
-- Headline numbers for the last @days days.
SELECT
    (SELECT COUNT(*) FROM mentions m
       WHERE m.workspace_id = @workspace_id
         AND m.created_at >= NOW() - make_interval(days => @days::int))::int AS mentions,
    (SELECT COUNT(*) FROM mentions m
       WHERE m.workspace_id = @workspace_id
         AND m.created_at >= NOW() - make_interval(days => @days::int)
         AND m.relevance_score >= 6.0
         AND m.intent IN ('buy_signal', 'recommendation_ask', 'complaint', 'comparison'))::int AS high_intent,
    (SELECT COUNT(*) FROM mentions m
       WHERE m.workspace_id = @workspace_id
         AND m.created_at >= NOW() - make_interval(days => @days::int)
         AND m.intent = 'complaint')::int AS complaints,
    (SELECT COUNT(*) FROM leads l
       WHERE l.workspace_id = @workspace_id
         AND l.created_at >= NOW() - make_interval(days => @days::int))::int AS new_leads,
    (SELECT COUNT(*) FROM replies r
       WHERE r.workspace_id = @workspace_id
         AND r.status = 'posted'
         AND r.created_at >= NOW() - make_interval(days => @days::int))::int AS replies_posted,
    (SELECT COALESCE(SUM(u.revenue_cents), 0) FROM utm_links u
       WHERE u.workspace_id = @workspace_id
         AND u.created_at >= NOW() - make_interval(days => @days::int))::bigint AS revenue_cents;
