-- +goose Up
-- enrich (P2 #16): LinkedIn / X profiles whose new posts become mentions.
-- Polled by the monitor through treg user-posts tools.
CREATE TABLE tracked_profiles (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id   UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    platform       TEXT NOT NULL CHECK (platform IN ('linkedin', 'x')),
    handle         TEXT NOT NULL,
    url            TEXT NOT NULL DEFAULT '',
    last_polled_at TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workspace_id, platform, handle)
);

CREATE INDEX idx_tracked_profiles_due ON tracked_profiles(last_polled_at NULLS FIRST);

-- +goose Down
DROP TABLE tracked_profiles;
