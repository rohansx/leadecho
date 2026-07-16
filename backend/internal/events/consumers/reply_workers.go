package consumers

import (
	"context"
	"encoding/json"

	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/events"
	streamredis "leadecho/internal/events/redis"
	"leadecho/internal/reply"
)

type ReplyWorkers struct {
	cfg     streamConfig
	drafter *reply.Drafter
}

func NewReplyWorkers(q *database.Queries, streams *streamredis.Client, drafter *reply.Drafter, logger zerolog.Logger, consumerName string, batchSize, blockMS, claimIdleMS, maxAttempts int64) *ReplyWorkers {
	return &ReplyWorkers{
		cfg:     newStreamConfig(q, streams, logger.With().Str("component", "reply_workers").Logger(), consumerName, batchSize, blockMS, claimIdleMS, maxAttempts),
		drafter: drafter,
	}
}

func (w *ReplyWorkers) StartDrafter(ctx context.Context) error {
	if err := w.cfg.streams.EnsureGroup(ctx, events.StreamReplyEvents, events.GroupReplyDrafters); err != nil {
		return err
	}
	return w.cfg.loop(ctx, events.StreamReplyEvents, events.GroupReplyDrafters, w.handleDraftRequested)
}

func (w *ReplyWorkers) handleDraftRequested(ctx context.Context, stream, group string, msg goredis.XMessage) error {
	env, err := streamredis.MessageToEnvelope(msg)
	if err != nil {
		return err
	}
	if env.EventType != events.EventTypeReplyDraftRequested {
		return nil
	}
	already, err := processedOrSkip(ctx, w.cfg.q, group, env.EventID)
	if err != nil {
		return err
	}
	if already {
		return nil
	}

	var payload events.ReplyDraftRequestedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return err
	}

	result, err := w.drafter.DraftForMention(ctx, payload.WorkspaceID, payload.MentionID)
	if err != nil {
		return err
	}
	if !result.ShouldReply {
		w.cfg.logger.Info().
			Str("mention_id", payload.MentionID).
			Str("reason", result.Reason).
			Msg("reply drafter: skipped mention")
	} else {
		w.cfg.logger.Info().
			Str("mention_id", payload.MentionID).
			Str("reply_id", result.Reply.ID).
			Str("source", payload.Source).
			Msg("reply drafter: draft created")
	}

	if err := markProcessed(ctx, w.cfg.q, group, w.cfg.consumerName, env.EventID, stream, msg.ID, payload.WorkspaceID); err != nil {
		return err
	}
	return nil
}

func (w *ReplyWorkers) Handler() messageHandler {
	return w.handleDraftRequested
}
