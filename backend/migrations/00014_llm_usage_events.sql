-- +goose Up
CREATE TABLE IF NOT EXISTS llm_usage_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    task TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('success', 'error')),
    prompt_tokens INT NOT NULL DEFAULT 0,
    completion_tokens INT NOT NULL DEFAULT 0,
    total_tokens INT NOT NULL DEFAULT 0,
    estimated_cost_usd NUMERIC(12, 6) NOT NULL DEFAULT 0,
    latency_ms INT NOT NULL DEFAULT 0,
    error_message TEXT,
    fallback_from TEXT,
    key_source TEXT NOT NULL DEFAULT 'unknown',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS llm_usage_events_workspace_created_idx
    ON llm_usage_events (workspace_id, created_at DESC);

CREATE INDEX IF NOT EXISTS llm_usage_events_workspace_task_idx
    ON llm_usage_events (workspace_id, task, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS llm_usage_events;
