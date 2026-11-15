-- +goose Up
-- competitor watch: the competitors of a workspace, where each one publishes,
-- what was fetched from there, and the moves read out of it. Nothing here is
-- a mention: competitor content never reaches lead scoring.
CREATE TABLE competitors (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    website      TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused')),
    summary      TEXT NOT NULL DEFAULT '',   -- what they are building, from stored moves only
    summary_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX competitors_workspace_name ON competitors (workspace_id, lower(name));

CREATE TABLE competitor_sources (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    competitor_id   UUID NOT NULL REFERENCES competitors(id) ON DELETE CASCADE,
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('x', 'linkedin', 'page_landing', 'page_pricing', 'changelog', 'blog', 'github')),
    url             TEXT NOT NULL,
    last_checked_at TIMESTAMPTZ,
    last_hash       TEXT NOT NULL DEFAULT '',  -- '' = no baseline yet
    last_text       TEXT NOT NULL DEFAULT '',  -- page kinds: the text behind last_hash
    failures        INTEGER NOT NULL DEFAULT 0,
    broken          BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (competitor_id, kind, url)
);
CREATE INDEX competitor_sources_due ON competitor_sources (last_checked_at NULLS FIRST) WHERE NOT broken;

CREATE TABLE competitor_items (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id     UUID NOT NULL REFERENCES competitor_sources(id) ON DELETE CASCADE,
    competitor_id UUID NOT NULL REFERENCES competitors(id) ON DELETE CASCADE,
    workspace_id  UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    item_key      TEXT NOT NULL,
    title         TEXT NOT NULL DEFAULT '',
    body          TEXT NOT NULL DEFAULT '',
    url           TEXT NOT NULL DEFAULT '',
    happened_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at  TIMESTAMPTZ,
    attempts      INTEGER NOT NULL DEFAULT 0,  -- failed reads by the model
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_id, item_key)
);
CREATE INDEX competitor_items_unprocessed ON competitor_items (created_at) WHERE processed_at IS NULL;

CREATE TABLE competitor_moves (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    competitor_id UUID NOT NULL REFERENCES competitors(id) ON DELETE CASCADE,
    workspace_id  UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    kind          TEXT NOT NULL CHECK (kind IN ('launch', 'pricing', 'positioning', 'news')),
    title         TEXT NOT NULL,
    summary       TEXT NOT NULL DEFAULT '',
    urls          TEXT[] NOT NULL DEFAULT '{}',
    happened_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    impact        TEXT NOT NULL DEFAULT '',
    action        TEXT NOT NULL DEFAULT '',
    theme_id      TEXT NOT NULL DEFAULT '',   -- demand theme id, '' = none
    importance    TEXT NOT NULL DEFAULT 'normal' CHECK (importance IN ('high', 'normal')),
    alerted_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX competitor_moves_workspace ON competitor_moves (workspace_id, happened_at DESC);

-- One row once automatic setup has run for a workspace, so it runs once.
CREATE TABLE competitor_setup (
    workspace_id UUID PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    unresolved   TEXT[] NOT NULL DEFAULT '{}',  -- names whose website could not be found
    setup_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE competitor_setup;
DROP TABLE competitor_moves;
DROP TABLE competitor_items;
DROP TABLE competitor_sources;
DROP TABLE competitors;
