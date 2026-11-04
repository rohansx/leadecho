-- +goose Up
-- Workspace API keys for the MCP server and the public REST API. Only the
-- SHA-256 of the key is stored; the plaintext is shown once at creation.
CREATE TABLE workspace_api_keys (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    prefix       TEXT NOT NULL,
    key_hash     TEXT NOT NULL UNIQUE,
    scopes       TEXT[] NOT NULL DEFAULT '{read}',
    created_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workspace_api_keys_workspace ON workspace_api_keys(workspace_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS workspace_api_keys;
