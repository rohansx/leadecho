-- name: ListKeywordCadences :many
SELECT id, term, platforms::text[] AS platforms, is_active, cadence
FROM keywords
WHERE workspace_id = @workspace_id
ORDER BY created_at DESC;

-- name: UpdateKeywordCadence :one
UPDATE keywords SET cadence = @cadence, updated_at = NOW()
WHERE id = @id AND workspace_id = @workspace_id
RETURNING id, cadence;

-- name: ListKeywordPlatformScans :many
SELECT keyword_id, platform, last_scanned_at
FROM keyword_platform_scans
WHERE workspace_id = @workspace_id;

-- name: UpsertKeywordPlatformScan :exec
INSERT INTO keyword_platform_scans (keyword_id, workspace_id, platform, last_scanned_at)
SELECT k.id, k.workspace_id, @platform, NOW()
FROM keywords k
WHERE k.id = @keyword_id AND k.workspace_id = @workspace_id
ON CONFLICT (keyword_id, platform) DO UPDATE SET last_scanned_at = NOW();
