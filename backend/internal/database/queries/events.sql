-- name: CreateEventLog :one
INSERT INTO event_log (
    event_id, stream_name, event_type, schema_version, workspace_id,
    aggregate_type, aggregate_id, producer, payload, idempotency_key,
    correlation_id, causation_id, trace_id, occurred_at, replay_of_event_id
) VALUES (
    @event_id, @stream_name, @event_type, @schema_version, @workspace_id,
    @aggregate_type, @aggregate_id, @producer, @payload, @idempotency_key,
    @correlation_id, @causation_id, @trace_id, @occurred_at, @replay_of_event_id
) RETURNING *;

-- name: MarkEventLogPublished :one
UPDATE event_log
SET redis_message_id = @redis_message_id,
    publish_status = @publish_status,
    publish_attempts = publish_attempts + 1,
    last_error = @last_error,
    updated_at = NOW()
WHERE event_id = @event_id
RETURNING *;

-- name: MarkEventLogPublishFailed :one
UPDATE event_log
SET publish_status = @publish_status,
    publish_attempts = publish_attempts + 1,
    last_error = @last_error,
    updated_at = NOW()
WHERE event_id = @event_id
RETURNING *;

-- name: GetEventLogByEventID :one
SELECT * FROM event_log
WHERE event_id = @event_id;

-- name: ListEventLogByWorkspace :many
SELECT * FROM event_log
WHERE workspace_id = @workspace_id
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: ListReplayableEvents :many
SELECT * FROM event_log
WHERE stream_name = @stream_name
  AND (@event_type = '' OR event_type = @event_type)
  AND (@workspace_id::uuid IS NULL OR workspace_id = @workspace_id)
  AND (@aggregate_type = '' OR aggregate_type = @aggregate_type)
  AND (@aggregate_id = '' OR aggregate_id = @aggregate_id)
  AND (@from_occurred_at::timestamptz IS NULL OR occurred_at >= @from_occurred_at)
  AND (@to_occurred_at::timestamptz IS NULL OR occurred_at <= @to_occurred_at)
ORDER BY occurred_at ASC
LIMIT @lim;

-- name: CreateConsumerProcessedEvent :one
INSERT INTO consumer_processed_events (
    consumer_group, consumer_name, event_id, stream_name, redis_message_id,
    workspace_id, result_status
) VALUES (
    @consumer_group, @consumer_name, @event_id, @stream_name, @redis_message_id,
    @workspace_id, @result_status
) RETURNING *;

-- name: HasConsumerProcessedEvent :one
SELECT EXISTS (
    SELECT 1
    FROM consumer_processed_events
    WHERE consumer_group = @consumer_group
      AND event_id = @event_id
) AS processed;

-- name: UpsertConsumerCheckpoint :one
INSERT INTO consumer_checkpoints (
    stream_name, consumer_group, consumer_name, last_redis_message_id,
    pending_count, last_error, heartbeat_at
) VALUES (
    @stream_name, @consumer_group, @consumer_name, @last_redis_message_id,
    @pending_count, @last_error, @heartbeat_at
)
ON CONFLICT (stream_name, consumer_group, consumer_name)
DO UPDATE SET
    last_redis_message_id = EXCLUDED.last_redis_message_id,
    pending_count = EXCLUDED.pending_count,
    last_error = EXCLUDED.last_error,
    heartbeat_at = EXCLUDED.heartbeat_at,
    updated_at = NOW()
RETURNING *;

-- name: ListConsumerCheckpoints :many
SELECT * FROM consumer_checkpoints
ORDER BY heartbeat_at DESC;

-- name: CreateDeadLetterEvent :one
INSERT INTO dead_letter_events (
    event_id, stream_name, consumer_group, consumer_name, workspace_id,
    redis_message_id, payload, error_code, error_class, error_message,
    stack_excerpt, retry_count, first_seen_at, last_seen_at, resolution_status
) VALUES (
    @event_id, @stream_name, @consumer_group, @consumer_name, @workspace_id,
    @redis_message_id, @payload, @error_code, @error_class, @error_message,
    @stack_excerpt, @retry_count, @first_seen_at, @last_seen_at, @resolution_status
) RETURNING *;

-- name: ListOpenDeadLetterEvents :many
SELECT * FROM dead_letter_events
WHERE resolution_status = 'open'
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: ResolveDeadLetterEvent :one
UPDATE dead_letter_events
SET resolution_status = @resolution_status,
    resolved_at = @resolved_at,
    last_seen_at = NOW()
WHERE id = @id
RETURNING *;

-- name: CreateEventReplay :one
INSERT INTO event_replays (
    requested_by, stream_name, consumer_group, event_type, workspace_id,
    aggregate_type, aggregate_id, from_occurred_at, to_occurred_at,
    replay_mode, status
) VALUES (
    @requested_by, @stream_name, @consumer_group, @event_type, @workspace_id,
    @aggregate_type, @aggregate_id, @from_occurred_at, @to_occurred_at,
    @replay_mode, @status
) RETURNING *;

-- name: UpdateEventReplayStatus :one
UPDATE event_replays
SET status = @status,
    replayed_count = @replayed_count,
    last_error = @last_error,
    updated_at = NOW()
WHERE id = @id
RETURNING *;

-- name: ListEventReplays :many
SELECT * FROM event_replays
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;
