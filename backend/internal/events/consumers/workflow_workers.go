package consumers

import (
	"context"
	"encoding/json"

	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/events"
	streamredis "leadecho/internal/events/redis"
	"leadecho/internal/workflow"
)

type WorkflowWorkers struct {
	cfg    streamConfig
	engine *workflow.Engine
}

func NewWorkflowWorkers(q *database.Queries, streams *streamredis.Client, engine *workflow.Engine, logger zerolog.Logger, consumerName string, batchSize, blockMS, claimIdleMS, maxAttempts int64) *WorkflowWorkers {
	return &WorkflowWorkers{
		cfg:    newStreamConfig(q, streams, logger.With().Str("component", "workflow_workers").Logger(), consumerName, batchSize, blockMS, claimIdleMS, maxAttempts),
		engine: engine,
	}
}

func (w *WorkflowWorkers) StartExecutor(ctx context.Context) error {
	if err := w.cfg.streams.EnsureGroup(ctx, events.StreamWorkflowEvents, events.GroupWorkflowExecutors); err != nil {
		return err
	}
	return w.cfg.loop(ctx, events.StreamWorkflowEvents, events.GroupWorkflowExecutors, w.handleTriggerRequested)
}

func (w *WorkflowWorkers) handleTriggerRequested(ctx context.Context, stream, group string, msg goredis.XMessage) error {
	env, err := streamredis.MessageToEnvelope(msg)
	if err != nil {
		return err
	}
	if env.EventType != events.EventTypeWorkflowTriggerRequested {
		return nil
	}
	already, err := processedOrSkip(ctx, w.cfg.q, group, env.EventID)
	if err != nil {
		return err
	}
	if already {
		return nil
	}

	var payload events.WorkflowTriggerRequestedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return err
	}

	if err := w.engine.EvaluateAndTrigger(ctx, payload); err != nil {
		return err
	}

	if err := markProcessed(ctx, w.cfg.q, group, w.cfg.consumerName, env.EventID, stream, msg.ID, payload.WorkspaceID); err != nil {
		return err
	}
	return nil
}

func (w *WorkflowWorkers) Handler() messageHandler {
	return w.handleTriggerRequested
}
