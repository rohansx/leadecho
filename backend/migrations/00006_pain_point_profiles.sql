-- +goose Up

-- Monitoring profiles: users describe problems their product solves
CREATE TABLE monitoring_profiles (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_monitoring_profiles_workspace ON monitoring_profiles(workspace_id) WHERE is_active = true;

CREATE TRIGGER monitoring_profiles_updated_at
    BEFORE UPDATE ON monitoring_profiles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Pain-point phrases with their Voyage AI embeddings (1024-dim)
CREATE TABLE pain_point_embeddings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id      UUID NOT NULL REFERENCES monitoring_profiles(id) ON DELETE CASCADE,
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    phrase          TEXT NOT NULL,
    embedding       vector(1024) NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_pain_point_embeddings_profile ON pain_point_embeddings(profile_id);
CREATE INDEX idx_pain_point_embeddings_workspace ON pain_point_embeddings(workspace_id);
CREATE INDEX idx_pain_point_embeddings_vector ON pain_point_embeddings
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Add content embedding + scoring metadata to mentions
ALTER TABLE mentions ADD COLUMN content_embedding vector(1024);
ALTER TABLE mentions ADD COLUMN scoring_metadata JSONB NOT NULL DEFAULT '{}';

CREATE INDEX idx_mentions_no_embedding ON mentions(workspace_id, created_at DESC)
    WHERE content_embedding IS NULL AND status != 'spam';

CREATE INDEX idx_mentions_content_embedding ON mentions
    USING hnsw (content_embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- +goose Down
DROP INDEX IF EXISTS idx_mentions_content_embedding;
DROP INDEX IF EXISTS idx_mentions_no_embedding;
ALTER TABLE mentions DROP COLUMN IF EXISTS scoring_metadata;
ALTER TABLE mentions DROP COLUMN IF EXISTS content_embedding;
DROP TABLE IF EXISTS pain_point_embeddings;
DROP TABLE IF EXISTS monitoring_profiles;
