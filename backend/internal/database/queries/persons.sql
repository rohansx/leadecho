-- name: CreatePerson :one
INSERT INTO persons (
    workspace_id, display_name, bio, company, location, icp_fit_score, confidence, metadata
) VALUES (
    @workspace_id, @display_name, @bio, @company, @location, @icp_fit_score, @confidence, @metadata
) RETURNING *;

-- name: UpdatePerson :one
UPDATE persons
SET
    display_name = COALESCE(@display_name, display_name),
    bio = COALESCE(@bio, bio),
    company = COALESCE(@company, company),
    location = COALESCE(@location, location),
    icp_fit_score = COALESCE(@icp_fit_score, icp_fit_score),
    confidence = COALESCE(@confidence, confidence),
    metadata = COALESCE(@metadata, metadata)
WHERE id = @id AND workspace_id = @workspace_id
RETURNING *;

-- name: GetPerson :one
SELECT * FROM persons
WHERE id = @id AND workspace_id = @workspace_id;

-- name: GetPersonByLead :one
SELECT p.* FROM persons p
JOIN leads l ON l.person_id = p.id
WHERE l.id = @lead_id AND l.workspace_id = @workspace_id;

-- name: LinkLeadToPerson :exec
UPDATE leads
SET person_id = @person_id
WHERE id = @lead_id AND workspace_id = @workspace_id;

-- name: UpsertPersonIdentity :one
INSERT INTO person_identities (
    person_id, workspace_id, platform, handle, profile_url, confidence, source, metadata
) VALUES (
    @person_id, @workspace_id, @platform, @handle, @profile_url, @confidence, @source, @metadata
)
ON CONFLICT (workspace_id, platform, handle) DO UPDATE SET
    person_id = EXCLUDED.person_id,
    profile_url = COALESCE(EXCLUDED.profile_url, person_identities.profile_url),
    confidence = GREATEST(person_identities.confidence, EXCLUDED.confidence),
    metadata = person_identities.metadata || EXCLUDED.metadata
RETURNING *;

-- name: ListPersonIdentities :many
SELECT * FROM person_identities
WHERE person_id = @person_id AND workspace_id = @workspace_id
ORDER BY confidence DESC, created_at ASC;

-- name: GetPersonIdentity :one
SELECT * FROM person_identities
WHERE workspace_id = @workspace_id AND platform = @platform AND handle = @handle;
