package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"leadecho/internal/events"
)

type Client struct {
	redis  *goredis.Client
	logger zerolog.Logger
}

func NewClient(redis *goredis.Client, logger zerolog.Logger) *Client {
	return &Client{redis: redis, logger: logger.With().Str("component", "stream_client").Logger()}
}

func (c *Client) Publish(ctx context.Context, stream string, env events.Envelope) (string, error) {
	if err := env.Validate(); err != nil {
		return "", err
	}

	values, err := envelopeToValues(env)
	if err != nil {
		return "", err
	}

	id, err := c.redis.XAdd(ctx, &goredis.XAddArgs{
		Stream: stream,
		Values: values,
	}).Result()
	if err != nil {
		return "", fmt.Errorf("publish event to %s: %w", stream, err)
	}
	return id, nil
}

func (c *Client) EnsureGroup(ctx context.Context, stream, group string) error {
	if err := c.redis.XGroupCreateMkStream(ctx, stream, group, "0").Err(); err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("ensure group %s on %s: %w", group, stream, err)
	}
	return nil
}

func (c *Client) ReadGroup(ctx context.Context, stream, group, consumer string, count int64, block time.Duration) ([]goredis.XMessage, error) {
	result, err := c.redis.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Count:    count,
		Block:    block,
		NoAck:    false,
	}).Result()
	if err == goredis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read group %s/%s: %w", stream, group, err)
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result[0].Messages, nil
}

func (c *Client) Ack(ctx context.Context, stream, group string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := c.redis.XAck(ctx, stream, group, ids...).Err(); err != nil {
		return fmt.Errorf("ack %s/%s: %w", stream, group, err)
	}
	return nil
}

func (c *Client) AutoClaim(ctx context.Context, stream, group, consumer string, minIdle time.Duration, start string, count int64) ([]goredis.XMessage, string, error) {
	res, next, err := c.redis.XAutoClaim(ctx, &goredis.XAutoClaimArgs{
		Stream:   stream,
		Group:    group,
		Consumer: consumer,
		MinIdle:  minIdle,
		Start:    start,
		Count:    count,
	}).Result()
	if err == goredis.Nil {
		return nil, next, nil
	}
	if err != nil {
		return nil, next, fmt.Errorf("autoclaim %s/%s: %w", stream, group, err)
	}
	return res, next, nil
}

func (c *Client) Pending(ctx context.Context, stream, group string) (*goredis.XPending, error) {
	pending, err := c.redis.XPending(ctx, stream, group).Result()
	if err != nil {
		return nil, fmt.Errorf("pending %s/%s: %w", stream, group, err)
	}
	return pending, nil
}

func envelopeToValues(env events.Envelope) (map[string]any, error) {
	return map[string]any{
		"event_id":        env.EventID,
		"event_type":      env.EventType,
		"schema_version":  strconv.Itoa(int(env.SchemaVersion)),
		"workspace_id":    env.WorkspaceID,
		"aggregate_type":  env.AggregateType,
		"aggregate_id":    env.AggregateID,
		"producer":        env.Producer,
		"payload":         string(env.Payload),
		"idempotency_key": env.IdempotencyKey,
		"correlation_id":  env.CorrelationID,
		"causation_id":    env.CausationID,
		"trace_id":        env.TraceID,
		"occurred_at":     env.OccurredAt.Format(time.RFC3339Nano),
	}, nil
}

func MessageToEnvelope(msg goredis.XMessage) (events.Envelope, error) {
	get := func(key string) string {
		if v, ok := msg.Values[key]; ok {
			return fmt.Sprint(v)
		}
		return ""
	}

	schemaVersion := int32(1)
	if v := get("schema_version"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return events.Envelope{}, fmt.Errorf("parse schema_version: %w", err)
		}
		schemaVersion = int32(n)
	}

	occurredAt, err := time.Parse(time.RFC3339Nano, get("occurred_at"))
	if err != nil {
		return events.Envelope{}, fmt.Errorf("parse occurred_at: %w", err)
	}

	rawPayload := json.RawMessage(get("payload"))
	env := events.Envelope{
		EventID:        get("event_id"),
		EventType:      get("event_type"),
		SchemaVersion:  schemaVersion,
		WorkspaceID:    get("workspace_id"),
		AggregateType:  get("aggregate_type"),
		AggregateID:    get("aggregate_id"),
		Producer:       get("producer"),
		Payload:        rawPayload,
		IdempotencyKey: get("idempotency_key"),
		CorrelationID:  get("correlation_id"),
		CausationID:    get("causation_id"),
		TraceID:        get("trace_id"),
		OccurredAt:     occurredAt,
	}
	return env, env.Validate()
}
