//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"

	"leadecho/internal/config"
	"leadecho/internal/database"
	"leadecho/internal/events"
	"leadecho/internal/events/publishers"
	streamredis "leadecho/internal/events/redis"
	"leadecho/internal/llm"
	"leadecho/internal/monitor"
	"leadecho/internal/reply"
	"leadecho/internal/streamworkers"
	"leadecho/internal/workflow"
)

func TestStreamPipeline_MentionIngestedToReplyDraft(t *testing.T) {
	e := newTestEnv(t)
	defer e.cancel()

	ctx := e.ctx
	logger := e.logger()
	queries := database.New(e.pgPool)

	// Reuse dev-seeded workspace + keyword from migrations/00002_seed_dev_data.sql.
	const (
		workspaceID = "00000000-0000-0000-0000-000000000001"
		keywordID   = "00000000-0000-0000-0000-000000000010"
		mentionID   = "00000000-0000-0000-0000-000000000097"
		workflowID  = "00000000-0000-0000-0000-000000000096"
	)

	seedMentionAndWorkflow(t, ctx, e.pgPool, workspaceID, keywordID, mentionID, workflowID)

	stub := &llm.Stub{}
	streamClient := streamredis.NewClient(e.redis, logger)
	publisher := publishers.New(queries, streamClient, logger)

	mon := monitor.New(
		queries,
		logger,
		"",
		stub,
		nil, nil, nil,
		nil,
		"",
		publisher,
		true,
		false,
		false,
		true,
	)

	replyDrafter := reply.NewDrafter(queries, stub, nil)
	workflowEngine := workflow.NewEngine(queries, publisher, logger)

	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()

	cfg := &config.Config{
		StreamsEnabled:                   true,
		StreamsScorerConsumerEnabled:     true,
		StreamsQualifierConsumerEnabled:  true,
		StreamsNotifierConsumerEnabled:   true,
		StreamsReplyDrafterConsumerEnabled: true,
		StreamsWorkflowConsumerEnabled:   true,
		StreamsRetryConsumerEnabled:      false,
		StreamsConsumerName:              "integration-test",
		StreamsBatchSize:                 20,
		StreamsBlockMS:                   300,
		StreamsClaimIdleMS:               60000,
		StreamsMaxAttempts:               8,
	}

	streamworkers.New(streamworkers.Options{
		Config:         cfg,
		Logger:         logger,
		Queries:        queries,
		StreamClient:   streamClient,
		Monitor:        mon,
		ReplyDrafter:   replyDrafter,
		WorkflowEngine: workflowEngine,
	}).Start(workerCtx)

	ingestEnv, err := events.NewEnvelope(
		events.EventTypeMentionIngested,
		events.AggregateTypeMention,
		mentionID,
		"integration_test",
		workspaceID,
		fmt.Sprintf("%s:%s", events.EventTypeMentionIngested, mentionID),
		events.MentionIngestedPayload{
			MentionID:   mentionID,
			WorkspaceID: workspaceID,
			Platform:    "reddit",
			Title:       "Looking for a CRM alternative",
			URL:         "https://reddit.com/r/SaaS/comments/test",
			Author:      "startup_sarah",
			Content:     "We need a CRM alternative that monitors Reddit and HN for buying signals. Budget around $100/mo for our small team.",
		},
	)
	if err != nil {
		t.Fatalf("build ingest envelope: %v", err)
	}
	if _, err := publisher.Publish(ctx, ingestEnv); err != nil {
		t.Fatalf("publish mention.ingested: %v", err)
	}

	waitFor(t, ctx, 45*time.Second, "mention.scored event", func() bool {
		return countRows(t, ctx, e.pgPool,
			`SELECT COUNT(*) FROM event_log WHERE event_type = $1 AND aggregate_id = $2`,
			events.EventTypeMentionScored, mentionID,
		) >= 1
	})

	waitFor(t, ctx, 45*time.Second, "scorer consumer processed", func() bool {
		return countRows(t, ctx, e.pgPool,
			`SELECT COUNT(*) FROM consumer_processed_events WHERE consumer_group = $1 AND event_id = $2`,
			events.GroupMentionScorers, ingestEnv.EventID,
		) >= 1
	})

	waitFor(t, ctx, 45*time.Second, "auto-qualified lead", func() bool {
		return countRows(t, ctx, e.pgPool,
			`SELECT COUNT(*) FROM leads WHERE workspace_id = $1 AND mention_id = $2`,
			workspaceID, mentionID,
		) >= 1
	})

	waitFor(t, ctx, 45*time.Second, "workflow execution", func() bool {
		return countRows(t, ctx, e.pgPool,
			`SELECT COUNT(*) FROM workflow_executions WHERE workflow_id = $1 AND mention_id = $2`,
			workflowID, mentionID,
		) >= 1
	})

	waitFor(t, ctx, 45*time.Second, "reply.draft_requested event", func() bool {
		return countRows(t, ctx, e.pgPool,
			`SELECT COUNT(*) FROM event_log WHERE event_type = $1`,
			events.EventTypeReplyDraftRequested,
		) >= 1
	})

	waitFor(t, ctx, 45*time.Second, "draft reply row", func() bool {
		return countRows(t, ctx, e.pgPool,
			`SELECT COUNT(*) FROM replies WHERE workspace_id = $1 AND mention_id = $2`,
			workspaceID, mentionID,
		) >= 1
	})

	if countRows(t, ctx, e.pgPool,
		`SELECT COUNT(*) FROM event_log WHERE event_type = $1`,
		events.EventTypeMentionQualified,
	) < 1 {
		t.Fatal("expected mention.qualified event")
	}

	if countRows(t, ctx, e.pgPool,
		`SELECT COUNT(*) FROM event_log WHERE event_type = $1 AND aggregate_id = $2`,
		events.EventTypeWorkflowTriggerRequested, mentionID,
	) < 1 {
		t.Fatal("expected workflow.trigger_requested event")
	}
}

func seedMentionAndWorkflow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, workspaceID, keywordID, mentionID, workflowID string) {
	t.Helper()

	var profileID string
	if err := pool.QueryRow(ctx, `
		SELECT id FROM monitoring_profiles WHERE workspace_id = $1 ORDER BY created_at ASC LIMIT 1
	`, workspaceID).Scan(&profileID); err != nil {
		t.Fatalf("lookup profile: %v", err)
	}

	vec := stubEmbeddingVector()
	if _, err := pool.Exec(ctx, `
		INSERT INTO pain_point_embeddings (profile_id, workspace_id, phrase, embedding)
		VALUES ($1, $2, 'CRM alternative monitoring', $3)
		ON CONFLICT DO NOTHING
	`, profileID, workspaceID, vec); err != nil {
		t.Fatalf("seed pain point embedding: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO mentions (
			id, workspace_id, keyword_id, platform, platform_id, url,
			title, content, author_username, status, platform_created_at
		) VALUES (
			$1, $2, $3, 'reddit', 'reddit_test_1', 'https://reddit.com/r/SaaS/comments/test',
			'Looking for a CRM alternative',
			'We need a CRM alternative that monitors Reddit and HN for buying signals. Budget around $100/mo for our small team.',
			'startup_sarah', 'new', NOW()
		)
		ON CONFLICT (id) DO NOTHING
	`, mentionID, workspaceID, keywordID); err != nil {
		t.Fatalf("seed mention: %v", err)
	}

	trigger, _ := json.Marshal(map[string]any{
		"platforms":    []string{"reddit"},
		"min_score":    7.0,
		"intent_types": []string{"buy_signal"},
	})
	actions, _ := json.Marshal([]map[string]any{
		{"type": "ai_draft", "config": map[string]any{}},
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO workflows (id, workspace_id, name, status, trigger_config, action_chain)
		VALUES ($1, $2, 'Integration Auto Draft', 'active', $3, $4)
		ON CONFLICT (id) DO NOTHING
	`, workflowID, workspaceID, trigger, actions); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
}

func stubEmbeddingVector() pgvector.Vector {
	v := make([]float32, 1024)
	v[0] = 0.25
	return pgvector.NewVector(v)
}
