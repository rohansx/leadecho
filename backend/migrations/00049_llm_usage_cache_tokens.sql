-- +goose Up
-- Prompt-cache tokens per call, so the cache hit rate shows per task (the
-- estimated cost already counts them). Constant defaults: metadata-only.
ALTER TABLE llm_usage_events
    ADD COLUMN IF NOT EXISTS cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cache_write_tokens INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE llm_usage_events DROP COLUMN cache_read_tokens, DROP COLUMN cache_write_tokens;
