-- +goose Up
-- Person360 foundation: persons, identities, lead linkage.

CREATE TABLE persons (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    display_name    TEXT,
    bio             TEXT,
    company         TEXT,
    location        TEXT,
    icp_fit_score   REAL,
    confidence      REAL NOT NULL DEFAULT 0,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE person_identities (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    person_id       UUID NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    platform        TEXT NOT NULL,
    handle          TEXT NOT NULL,
    profile_url     TEXT,
    confidence      REAL NOT NULL DEFAULT 0,
    source          TEXT NOT NULL DEFAULT 'explicit',
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workspace_id, platform, handle)
);

CREATE INDEX idx_persons_workspace ON persons(workspace_id, created_at DESC);
CREATE INDEX idx_person_identities_person ON person_identities(person_id);
CREATE INDEX idx_person_identities_ws ON person_identities(workspace_id, platform);

ALTER TABLE leads ADD COLUMN person_id UUID REFERENCES persons(id) ON DELETE SET NULL;
CREATE INDEX idx_leads_person ON leads(person_id) WHERE person_id IS NOT NULL;

CREATE TRIGGER persons_updated_at
    BEFORE UPDATE ON persons
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- +goose Down
ALTER TABLE leads DROP COLUMN IF EXISTS person_id;
DROP TABLE IF EXISTS person_identities;
DROP TABLE IF EXISTS persons;
