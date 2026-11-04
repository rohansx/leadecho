-- +goose Up
-- sources: Bluesky monitor, mute lists, per-platform scan cadence.

-- Bluesky as a monitoring source (public AT Protocol search, no key needed).
-- Not used elsewhere in this migration, so it is safe inside the transaction.
ALTER TYPE platform_type ADD VALUE IF NOT EXISTS 'bluesky';

-- Mute lists: mentions matching any mute are dropped before insert.
CREATE TABLE mutes (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('author', 'domain', 'keyword', 'subreddit')),
    value        TEXT NOT NULL CHECK (value <> ''),
    created_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX mutes_workspace_kind_value_key ON mutes (workspace_id, kind, lower(value));

-- Per-platform cadence, e.g. {"twitter":"1h","reddit":"30m"}. Missing = hourly.
ALTER TABLE keywords ADD COLUMN cadence JSONB NOT NULL DEFAULT '{}';

-- Last scan time per keyword+platform, consulted by the monitor tick.
CREATE TABLE keyword_platform_scans (
    keyword_id      UUID NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    platform        TEXT NOT NULL,
    last_scanned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (keyword_id, platform)
);
CREATE INDEX keyword_platform_scans_workspace_idx ON keyword_platform_scans (workspace_id);

-- +goose Down
DROP TABLE IF EXISTS keyword_platform_scans;
ALTER TABLE keywords DROP COLUMN IF EXISTS cadence;
DROP TABLE IF EXISTS mutes;
-- Postgres cannot drop an enum value; 'bluesky' stays (see 00023).
