//go:build integration

package integration

import (
	"fmt"
	"testing"
	"time"

	"leadecho/internal/database"
	"leadecho/internal/events"
	"leadecho/internal/events/publishers"
	streamredis "leadecho/internal/events/redis"
	"leadecho/internal/llm"
	"leadecho/internal/monitor"
)

// TestDualWrite_Phase1 verifies Phase 1 rollout:
// events are published while inline scoring still runs, with no consumers enabled.
func TestDualWrite_Phase1(t *testing.T) {
	e := newTestEnv(t)
	defer e.cancel()

	ctx := e.ctx
	logger := e.logger()
	queries := database.New(e.pgPool)

	const (
		workspaceID = "00000000-0000-0000-0000-000000000001"
		keywordID   = "00000000-0000-0000-0000-000000000010"
		mentionID   = "00000000-0000-0000-0000-000000000098"
	)

	seedMentionAndWorkflow(t, ctx, e.pgPool, workspaceID, keywordID, mentionID, "00000000-0000-0000-0000-000000000095")

	streamClient := streamredis.NewClient(e.redis, logger)
	publisher := publishers.New(queries, streamClient, logger)

	mon := monitor.New(
		queries,
		logger,
		"",
		&llm.Stub{},
		nil, nil, nil,
		nil,
		"",
		publisher,
		true, // streamsEnabled
		true, // dualWrite
		true, // inlineFallback
		false,
		nil,
	)

	beforeEvents := countRows(t, ctx, e.pgPool, `SELECT COUNT(*) FROM event_log`)
	beforeStreamLen, err := e.redis.XLen(ctx, events.StreamMentionEvents).Result()
	if err != nil {
		t.Fatalf("redis xlen: %v", err)
	}

	mon.IngestSignals(ctx, workspaceID, []monitor.SignalAlert{{
		ID:       mentionID,
		Platform: "reddit",
		Title:    "Looking for a CRM alternative",
		URL:      "https://reddit.com/r/SaaS/comments/dual-write-test",
		Author:   "startup_sarah",
		Content:  "We need a CRM alternative that monitors Reddit and HN for buying signals. Budget around $100/mo for our small team.",
	}})

	waitFor(t, ctx, 15*time.Second, "mention.ingested in event_log", func() bool {
		return countRows(t, ctx, e.pgPool,
			`SELECT COUNT(*) FROM event_log WHERE event_type = $1 AND aggregate_id = $2`,
			events.EventTypeMentionIngested, mentionID,
		) >= 1
	})

	waitFor(t, ctx, 15*time.Second, "inline mention.scored in event_log", func() bool {
		return countRows(t, ctx, e.pgPool,
			`SELECT COUNT(*) FROM event_log WHERE event_type = $1 AND aggregate_id = $2`,
			events.EventTypeMentionScored, mentionID,
		) >= 1
	})

	afterStreamLen, err := e.redis.XLen(ctx, events.StreamMentionEvents).Result()
	if err != nil {
		t.Fatalf("redis xlen after: %v", err)
	}
	if afterStreamLen <= beforeStreamLen {
		t.Fatalf("expected redis stream to grow: before=%d after=%d", beforeStreamLen, afterStreamLen)
	}

	afterEvents := countRows(t, ctx, e.pgPool, `SELECT COUNT(*) FROM event_log`)
	if afterEvents <= beforeEvents {
		t.Fatalf("expected event_log to grow: before=%d after=%d", beforeEvents, afterEvents)
	}

	mention, err := queries.GetMention(ctx, database.GetMentionParams{
		ID:          mentionID,
		WorkspaceID: workspaceID,
	})
	if err != nil {
		t.Fatalf("get mention: %v", err)
	}
	if !mention.Intent.Valid || mention.Intent.IntentType == "" {
		t.Fatal("expected inline scorer to set mention intent")
	}
	if !mention.RelevanceScore.Valid || mention.RelevanceScore.Float32 <= 0 {
		t.Fatal("expected inline scorer to set relevance_score")
	}

	processed := countRows(t, ctx, e.pgPool, `SELECT COUNT(*) FROM consumer_processed_events`)
	if processed != 0 {
		t.Fatalf("phase 1 should not run consumers; got %d processed events", processed)
	}

	published := countRows(t, ctx, e.pgPool,
		`SELECT COUNT(*) FROM event_log WHERE publish_status = 'published' AND aggregate_id = $1`,
		mentionID,
	)
	if published < 1 {
		t.Fatal("expected at least one published event for mention")
	}

	t.Logf("dual-write ok: event_log +%d, stream +%d, intent=%s score=%.1f",
		afterEvents-beforeEvents,
		afterStreamLen-beforeStreamLen,
		mention.Intent.IntentType,
		mention.RelevanceScore.Float32,
	)
}

func TestDualWrite_IdempotentPublishKey(t *testing.T) {
	e := newTestEnv(t)
	defer e.cancel()

	ctx := e.ctx
	queries := database.New(e.pgPool)
	publisher := publishers.New(queries, streamredis.NewClient(e.redis, e.logger()), e.logger())

	mentionID := "00000000-0000-0000-0000-000000000094"
	workspaceID := "00000000-0000-0000-0000-000000000001"

	env, err := events.NewEnvelope(
		events.EventTypeMentionIngested,
		events.AggregateTypeMention,
		mentionID,
		"test",
		workspaceID,
		fmt.Sprintf("%s:%s", events.EventTypeMentionIngested, mentionID),
		events.MentionIngestedPayload{MentionID: mentionID, WorkspaceID: workspaceID, Platform: "reddit", URL: "https://example.com", Content: "test content long enough for rules filter to pass easily here."},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := publisher.Publish(ctx, env); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if _, err := publisher.Publish(ctx, env); err == nil {
		t.Fatal("expected duplicate idempotency key to fail")
	}
}
