-- name: MentionsPerDay :many
SELECT DATE(created_at) as day, COUNT(*)::int as count
FROM mentions
WHERE workspace_id = @workspace_id
AND created_at >= NOW() - INTERVAL '30 days'
GROUP BY DATE(created_at)
ORDER BY day;

-- name: MentionsPerPlatform :many
SELECT platform, COUNT(*)::int as count
FROM mentions
WHERE workspace_id = @workspace_id
AND created_at >= NOW() - INTERVAL '30 days'
GROUP BY platform;

-- name: MentionsPerIntent :many
SELECT intent, COUNT(*)::int as count
FROM mentions
WHERE workspace_id = @workspace_id
AND intent IS NOT NULL
AND created_at >= NOW() - INTERVAL '30 days'
GROUP BY intent;

-- name: TopKeywords :many
SELECT k.term, COUNT(m.id)::int as mention_count
FROM keywords k
LEFT JOIN mentions m ON (m.keyword_id = k.id OR k.term = ANY(m.keyword_matches))
AND m.workspace_id = k.workspace_id
AND m.created_at >= NOW() - INTERVAL '30 days'
WHERE k.workspace_id = @workspace_id
GROUP BY k.id, k.term
ORDER BY mention_count DESC
LIMIT 10;

-- name: ConversionFunnel :many
SELECT stage, COUNT(*)::int as count
FROM leads
WHERE workspace_id = @workspace_id
GROUP BY stage;

-- name: ReplyStats :many
SELECT status, COUNT(*)::int as count
FROM replies
WHERE workspace_id = @workspace_id
AND created_at >= NOW() - INTERVAL '30 days'
GROUP BY status;

-- name: CountMentions30d :one
SELECT COUNT(*)::int as count FROM mentions
WHERE workspace_id = @workspace_id AND created_at >= NOW() - INTERVAL '30 days';

-- name: CountNewMentions :one
SELECT COUNT(*)::int as count FROM mentions
WHERE workspace_id = @workspace_id AND status = 'new';

-- name: CountTotalLeads :one
SELECT COUNT(*)::int as count FROM leads
WHERE workspace_id = @workspace_id;

-- name: CountConvertedLeads :one
SELECT COUNT(*)::int as count FROM leads
WHERE workspace_id = @workspace_id AND stage = 'converted';

-- name: CountRepliesPosted30d :one
SELECT COUNT(*)::int as count FROM replies
WHERE workspace_id = @workspace_id AND status = 'posted'
AND created_at >= NOW() - INTERVAL '30 days';

-- name: CountActiveKeywords :one
SELECT COUNT(*)::int as count FROM keywords
WHERE workspace_id = @workspace_id AND is_active = true;
