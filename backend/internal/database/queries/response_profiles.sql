-- name: ListResponseProfiles :many
SELECT * FROM response_profiles
WHERE workspace_id = @workspace_id
ORDER BY is_default DESC, created_at;

-- name: GetResponseProfile :one
SELECT * FROM response_profiles
WHERE id = @id AND workspace_id = @workspace_id;

-- name: GetPreferredResponseProfile :one
SELECT * FROM response_profiles
WHERE workspace_id = @workspace_id
ORDER BY is_default DESC, created_at
LIMIT 1;

-- name: CreateResponseProfile :one
-- The first profile in a workspace becomes its default.
INSERT INTO response_profiles (
    workspace_id, name, goal, special_instructions, tone, length, assertiveness,
    brand_plug, cta_goal, destination_url, examples, is_default
) VALUES (
    @workspace_id, @name, @goal, @special_instructions, @tone, @length, @assertiveness,
    @brand_plug, @cta_goal, @destination_url, @examples,
    NOT EXISTS (SELECT 1 FROM response_profiles rp WHERE rp.workspace_id = @workspace_id)
) RETURNING *;

-- name: UpdateResponseProfile :one
UPDATE response_profiles
SET name = @name, goal = @goal, special_instructions = @special_instructions,
    tone = @tone, length = @length, assertiveness = @assertiveness,
    brand_plug = @brand_plug, cta_goal = @cta_goal, destination_url = @destination_url,
    examples = @examples
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: DeleteResponseProfile :execrows
DELETE FROM response_profiles
WHERE id = @id AND workspace_id = @workspace_id;

-- name: SetDefaultResponseProfile :execrows
-- One statement flips the old default off and the new one on; no-op when the
-- id is not in this workspace.
UPDATE response_profiles
SET is_default = (id = @id::uuid)
WHERE response_profiles.workspace_id = @workspace_id::uuid
  AND (is_default OR id = @id::uuid)
  AND EXISTS (SELECT 1 FROM response_profiles t WHERE t.id = @id::uuid AND t.workspace_id = @workspace_id::uuid);

-- name: SetReplyTrackingLink :one
UPDATE replies
SET content = @content, utm_link_id = @utm_link_id
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;
