-- +goose Up
-- One editable brand profile per workspace: the full product analysis that
-- onboarding used to drop (competitors, features, audience, segments,
-- grouped keywords). Feeds listener building and reply grounding.
CREATE TABLE brand_profiles (
    workspace_id      UUID PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    overview          TEXT NOT NULL DEFAULT '',
    product           TEXT NOT NULL DEFAULT '',
    positioning       TEXT NOT NULL DEFAULT '',
    customer_segments JSONB NOT NULL DEFAULT '[]', -- [{name, why_they_buy}]
    competitors       TEXT[] NOT NULL DEFAULT '{}',
    pain_points       JSONB NOT NULL DEFAULT '[]', -- [{problem, explanation}]
    keywords          JSONB NOT NULL DEFAULT '{}', -- {brand, product, category, problem}
    features          TEXT[] NOT NULL DEFAULT '{}',
    target_audience   TEXT NOT NULL DEFAULT '',
    source_url        TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER brand_profiles_updated_at
    BEFORE UPDATE ON brand_profiles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Thumbs up/down on listener-builder sample posts. Rows are keyed by goal
-- until onboarding completes, then linked to the created profile. These are
-- the first few-shot calibration examples for a listener.
CREATE TABLE onboarding_sample_feedback (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    profile_id   UUID REFERENCES monitoring_profiles(id) ON DELETE CASCADE,
    goal         TEXT NOT NULL,
    platform     TEXT NOT NULL DEFAULT '',
    url          TEXT NOT NULL,
    text         TEXT NOT NULL,
    label        TEXT NOT NULL CHECK (label IN ('up', 'down')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workspace_id, goal, url)
);

CREATE INDEX idx_onboarding_sample_feedback_profile
    ON onboarding_sample_feedback(workspace_id, profile_id);

-- +goose Down
DROP TABLE onboarding_sample_feedback;
DROP TABLE brand_profiles;
