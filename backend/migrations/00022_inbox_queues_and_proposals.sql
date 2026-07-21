-- +goose Up
-- Human proposals queue (Discovery agent output) + conversion webhook secret storage via settings JSONB (no schema change).

CREATE TABLE human_proposals (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    proposal_type   TEXT NOT NULL DEFAULT 'community',
    title           TEXT NOT NULL,
    body            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_human_proposals_ws_status ON human_proposals(workspace_id, status, created_at DESC);

CREATE TRIGGER human_proposals_updated_at
    BEFORE UPDATE ON human_proposals
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

ALTER TABLE replies ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE replies DROP COLUMN IF EXISTS metadata;
DROP TABLE IF EXISTS human_proposals;
