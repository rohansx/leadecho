-- +goose Up

CREATE TABLE extension_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    token        TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL DEFAULT 'Default',
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_extension_tokens_workspace ON extension_tokens(workspace_id);
CREATE INDEX idx_extension_tokens_token ON extension_tokens(token);

-- +goose Down

DROP TABLE IF EXISTS extension_tokens;
