-- name: ListHumanProposals :many
SELECT * FROM human_proposals
WHERE workspace_id = @workspace_id AND status = @status
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: CountHumanProposalsByStatus :many
SELECT status, COUNT(*)::int AS count
FROM human_proposals
WHERE workspace_id = @workspace_id
GROUP BY status;

-- name: CreateHumanProposal :one
INSERT INTO human_proposals (
    workspace_id, proposal_type, title, body, status, metadata
) VALUES (
    @workspace_id, @proposal_type, @title, @body, @status, @metadata
) RETURNING *;

-- name: UpdateHumanProposalStatus :one
UPDATE human_proposals
SET status = @status
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;
