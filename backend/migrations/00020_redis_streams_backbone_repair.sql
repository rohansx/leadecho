-- +goose Up
-- Idempotent repair for databases where an older migration already consumed version 14.

CREATE TABLE IF NOT EXISTS event_log (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id            UUID NOT NULL UNIQUE,
    stream_name         TEXT NOT NULL,
    event_type          TEXT NOT NULL,
    schema_version      INTEGER NOT NULL DEFAULT 1,
    workspace_id        UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    aggregate_type      TEXT NOT NULL,
    aggregate_id        TEXT NOT NULL,
    producer            TEXT NOT NULL,
    payload             JSONB NOT NULL,
    idempotency_key     TEXT NOT NULL,
    correlation_id      TEXT,
    causation_id        TEXT,
    trace_id            TEXT,
    occurred_at         TIMESTAMPTZ NOT NULL,
    redis_message_id    TEXT,
    publish_status      TEXT NOT NULL DEFAULT 'pending',
    publish_attempts    INTEGER NOT NULL DEFAULT 0,
    last_error          TEXT,
    replay_of_event_id  UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(stream_name, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_event_log_stream_created ON event_log(stream_name, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_event_log_workspace_type ON event_log(workspace_id, event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_event_log_publish_status ON event_log(publish_status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_event_log_aggregate ON event_log(aggregate_type, aggregate_id, created_at DESC);

DROP TRIGGER IF EXISTS event_log_updated_at ON event_log;
CREATE TRIGGER event_log_updated_at
    BEFORE UPDATE ON event_log
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TABLE IF NOT EXISTS consumer_processed_events (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    consumer_group      TEXT NOT NULL,
    consumer_name       TEXT NOT NULL,
    event_id            UUID NOT NULL,
    stream_name         TEXT NOT NULL,
    redis_message_id    TEXT NOT NULL,
    workspace_id        UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    processed_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    result_status       TEXT NOT NULL DEFAULT 'processed',
    UNIQUE(consumer_group, event_id)
);

CREATE INDEX IF NOT EXISTS idx_consumer_processed_stream ON consumer_processed_events(stream_name, processed_at DESC);
CREATE INDEX IF NOT EXISTS idx_consumer_processed_workspace ON consumer_processed_events(workspace_id, processed_at DESC);

CREATE TABLE IF NOT EXISTS dead_letter_events (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id            UUID NOT NULL,
    stream_name         TEXT NOT NULL,
    consumer_group      TEXT NOT NULL,
    consumer_name       TEXT NOT NULL,
    workspace_id        UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    redis_message_id    TEXT,
    payload             JSONB NOT NULL,
    error_code          TEXT,
    error_class         TEXT NOT NULL,
    error_message       TEXT NOT NULL,
    stack_excerpt       TEXT,
    retry_count         INTEGER NOT NULL DEFAULT 0,
    first_seen_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at         TIMESTAMPTZ,
    resolution_status   TEXT NOT NULL DEFAULT 'open',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dead_letter_open ON dead_letter_events(resolution_status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dead_letter_workspace ON dead_letter_events(workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dead_letter_event ON dead_letter_events(event_id, consumer_group);

CREATE TABLE IF NOT EXISTS consumer_checkpoints (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stream_name         TEXT NOT NULL,
    consumer_group      TEXT NOT NULL,
    consumer_name       TEXT NOT NULL,
    last_redis_message_id TEXT,
    pending_count       BIGINT NOT NULL DEFAULT 0,
    last_error          TEXT,
    heartbeat_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(stream_name, consumer_group, consumer_name)
);

CREATE INDEX IF NOT EXISTS idx_consumer_checkpoints_heartbeat ON consumer_checkpoints(heartbeat_at DESC);

DROP TRIGGER IF EXISTS consumer_checkpoints_updated_at ON consumer_checkpoints;
CREATE TRIGGER consumer_checkpoints_updated_at
    BEFORE UPDATE ON consumer_checkpoints
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TABLE IF NOT EXISTS event_replays (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    requested_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    stream_name         TEXT NOT NULL,
    consumer_group      TEXT,
    event_type          TEXT,
    workspace_id        UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    aggregate_type      TEXT,
    aggregate_id        TEXT,
    from_occurred_at    TIMESTAMPTZ,
    to_occurred_at      TIMESTAMPTZ,
    replay_mode         TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending',
    replayed_count      INTEGER NOT NULL DEFAULT 0,
    last_error          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_event_replays_status ON event_replays(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_event_replays_workspace ON event_replays(workspace_id, created_at DESC);

DROP TRIGGER IF EXISTS event_replays_updated_at ON event_replays;
CREATE TRIGGER event_replays_updated_at
    BEFORE UPDATE ON event_replays
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- +goose Down
-- Repair migration: no down (tables owned by 00014 when present).
