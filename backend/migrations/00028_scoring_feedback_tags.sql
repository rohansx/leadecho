-- +goose Up
-- scoring: 13-tag intent taxonomy + sentiment, dedupe keys, human feedback.

ALTER TABLE mentions ADD COLUMN intent_tags TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE mentions ADD COLUMN sentiment TEXT
    CHECK (sentiment IN ('positive', 'neutral', 'negative'));
-- Dedupe keys: normalized URL and a 64-bit simhash of normalized text.
ALTER TABLE mentions ADD COLUMN canonical_url TEXT;
ALTER TABLE mentions ADD COLUMN content_simhash BIGINT;

-- Map the old single intent onto the taxonomy so filters work on old rows.
UPDATE mentions SET intent_tags = CASE intent
    WHEN 'buy_signal' THEN ARRAY['buying_intent']
    WHEN 'recommendation_ask' THEN ARRAY['recommendation_request']
    WHEN 'complaint' THEN ARRAY['competitor_complaint']
    WHEN 'comparison' THEN ARRAY['comparison']
    ELSE '{}'::text[] END
WHERE intent IS NOT NULL;

CREATE INDEX idx_mentions_intent_tags ON mentions USING GIN (intent_tags);
CREATE INDEX idx_mentions_ws_canonical_url ON mentions (workspace_id, canonical_url)
    WHERE canonical_url IS NOT NULL;
CREATE INDEX idx_mentions_ws_created ON mentions (workspace_id, created_at DESC);

-- One vote per user per mention; the latest vote wins (NULL user = dev/no-auth).
-- NULLS NOT DISTINCT needs PG15+ (all stacks run pgvector:pg16).
CREATE TABLE mention_feedback (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    mention_id   UUID NOT NULL REFERENCES mentions(id) ON DELETE CASCADE,
    profile_id   UUID REFERENCES monitoring_profiles(id) ON DELETE SET NULL,
    user_id      UUID REFERENCES users(id) ON DELETE CASCADE,
    label        TEXT NOT NULL CHECK (label IN ('up', 'down')),
    note         TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (mention_id, user_id)
);
CREATE INDEX idx_mention_feedback_profile ON mention_feedback (workspace_id, profile_id, updated_at DESC);

-- +goose Down
DROP TABLE IF EXISTS mention_feedback;
DROP INDEX IF EXISTS idx_mentions_ws_created;
DROP INDEX IF EXISTS idx_mentions_ws_canonical_url;
DROP INDEX IF EXISTS idx_mentions_intent_tags;
ALTER TABLE mentions DROP COLUMN content_simhash;
ALTER TABLE mentions DROP COLUMN canonical_url;
ALTER TABLE mentions DROP COLUMN sentiment;
ALTER TABLE mentions DROP COLUMN intent_tags;
