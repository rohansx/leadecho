-- +goose Up
-- Response profiles (reply voice): goal preset + sliders + per-platform examples.
-- Replies never require one; the drafter derives a default when none exist.

CREATE TABLE response_profiles (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id         UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name                 TEXT NOT NULL,
    goal                 TEXT NOT NULL DEFAULT 'drive_conversion'
        CHECK (goal IN ('drive_conversion', 'build_relationship', 'ask_for_info', 'handle_feedback', 'support', 'custom')),
    special_instructions TEXT NOT NULL DEFAULT '',
    tone                 INT NOT NULL DEFAULT 40 CHECK (tone BETWEEN 0 AND 100),
    length               TEXT NOT NULL DEFAULT 'medium' CHECK (length IN ('short', 'medium', 'long')),
    assertiveness        INT NOT NULL DEFAULT 40 CHECK (assertiveness BETWEEN 0 AND 100),
    brand_plug           TEXT NOT NULL DEFAULT 'low' CHECK (brand_plug IN ('none', 'low', 'medium', 'high')),
    cta_goal             TEXT NOT NULL DEFAULT '',
    destination_url      TEXT NOT NULL DEFAULT '',
    examples             JSONB NOT NULL DEFAULT '{}',
    is_default           BOOLEAN NOT NULL DEFAULT FALSE,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ponytail: no unique index on the default. The drafter picks is_default DESC,
-- created_at, so a duplicate default is harmless and set-default stays one UPDATE.
CREATE INDEX idx_response_profiles_ws ON response_profiles(workspace_id, is_default DESC, created_at);

CREATE TRIGGER response_profiles_updated_at
    BEFORE UPDATE ON response_profiles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- +goose Down
DROP TABLE IF EXISTS response_profiles;
