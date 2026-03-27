-- +goose Up

-- Store scraped product analysis results for the zero-config onboarding wizard.
CREATE TABLE onboarding_analyses (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    source_url      TEXT NOT NULL,
    raw_text        TEXT,
    analysis        JSONB NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'pending',
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_onboarding_analyses_ws ON onboarding_analyses(workspace_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS onboarding_analyses;
