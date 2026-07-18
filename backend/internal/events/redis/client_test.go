package redis

import (
	"testing"
	"time"

	redislib "github.com/redis/go-redis/v9"

	"leadecho/internal/events"
)

func TestMessageToEnvelopeParsesPublishedFields(t *testing.T) {
	msg := redislib.XMessage{
		ID: "1720000000000-0",
		Values: map[string]any{
			"event_id":        "123e4567-e89b-12d3-a456-426614174000",
			"event_type":      events.EventTypeMentionScored,
			"schema_version":  "1",
			"workspace_id":    "workspace-123",
			"aggregate_type":  events.AggregateTypeMention,
			"aggregate_id":    "mention-123",
			"producer":        "scorer",
			"payload":         `{"mention_id":"mention-123"}`,
			"idempotency_key": "mention.scored:mention-123",
			"occurred_at":     time.Now().UTC().Format(time.RFC3339Nano),
		},
	}

	env, err := MessageToEnvelope(msg)
	if err != nil {
		t.Fatalf("MessageToEnvelope() error = %v", err)
	}
	if env.AggregateID != "mention-123" {
		t.Fatalf("unexpected aggregate id: %s", env.AggregateID)
	}
	if env.EventType != events.EventTypeMentionScored {
		t.Fatalf("unexpected event type: %s", env.EventType)
	}
}
