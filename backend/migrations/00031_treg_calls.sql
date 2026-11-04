-- +goose Up
-- treg_calls: our side of treg's billing ledger. One row per treg HTTP
-- response, keyed by X-Treg-Call-Id so it joins to treg's /calls/<id>.
CREATE TABLE IF NOT EXISTS treg_calls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    tool TEXT NOT NULL,
    call_id TEXT NOT NULL DEFAULT '',
    cost_micro BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS treg_calls_workspace_created_idx
    ON treg_calls (workspace_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS treg_calls;
