package consumers

import (
	"context"
	"encoding/json"

	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/events"
	streamredis "leadecho/internal/events/redis"
	"leadecho/internal/monitor"
)

type MentionWorkers struct {
	cfg streamConfig
	mon *monitor.Monitor
}

func NewMentionWorkers(q *database.Queries, streams *streamredis.Client, mon *monitor.Monitor, logger zerolog.Logger, consumerName string, batchSize, blockMS, claimIdleMS, maxAttempts int64) *MentionWorkers {
	return &MentionWorkers{
		cfg: newStreamConfig(q, streams, logger.With().Str("component", "mention_workers").Logger(), consumerName, batchSize, blockMS, claimIdleMS, maxAttempts),
		mon: mon,
	}
}

func (w *MentionWorkers) StartScorer(ctx context.Context) error {
	if err := w.cfg.streams.EnsureGroup(ctx, events.StreamMentionEvents, events.GroupMentionScorers); err != nil {
		return err
	}
	return w.cfg.loop(ctx, events.StreamMentionEvents, events.GroupMentionScorers, w.handleScorerMessage)
}

func (w *MentionWorkers) StartNotifier(ctx context.Context) error {
	if err := w.cfg.streams.EnsureGroup(ctx, events.StreamMentionEvents, events.GroupMentionNotifiers); err != nil {
		return err
	}
	return w.cfg.loop(ctx, events.StreamMentionEvents, events.GroupMentionNotifiers, w.handleNotifierMessage)
}

func (w *MentionWorkers) StartQualifier(ctx context.Context) error {
	if err := w.cfg.streams.EnsureGroup(ctx, events.StreamMentionEvents, events.GroupMentionQualifiers); err != nil {
		return err
	}
	return w.cfg.loop(ctx, events.StreamMentionEvents, events.GroupMentionQualifiers, w.handleQualifierMessage)
}

func (w *MentionWorkers) handleScorerMessage(ctx context.Context, stream, group string, msg goredis.XMessage) error {
	env, err := streamredis.MessageToEnvelope(msg)
	if err != nil {
		return err
	}
	if env.EventType != events.EventTypeMentionIngested {
		return nil
	}
	already, err := processedOrSkip(ctx, w.cfg.q, group, env.EventID)
	if err != nil {
		return err
	}
	if already {
		return nil
	}

	var payload events.MentionIngestedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return err
	}
	w.mon.ScoreMentionBatch(ctx, payload.WorkspaceID, []monitor.SignalAlert{{
		ID:       payload.MentionID,
		Platform: payload.Platform,
		Title:    payload.Title,
		URL:      payload.URL,
		Author:   payload.Author,
		Content:  payload.Content,
	}})

	if err := markProcessed(ctx, w.cfg.q, group, w.cfg.consumerName, env.EventID, stream, msg.ID, payload.WorkspaceID); err != nil {
		return err
	}
	return nil
}

func (w *MentionWorkers) handleNotifierMessage(ctx context.Context, stream, group string, msg goredis.XMessage) error {
	env, err := streamredis.MessageToEnvelope(msg)
	if err != nil {
		return err
	}
	if env.EventType != events.EventTypeMentionNotificationRequest {
		return nil
	}
	already, err := processedOrSkip(ctx, w.cfg.q, group, env.EventID)
	if err != nil {
		return err
	}
	if already {
		return nil
	}

	var payload events.MentionNotificationRequestedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return err
	}
	mention, err := w.cfg.q.GetMention(ctx, database.GetMentionParams{ID: payload.MentionID, WorkspaceID: payload.WorkspaceID})
	if err != nil {
		return err
	}
	title := ""
	if mention.Title.Valid {
		title = mention.Title.String
	}
	author := ""
	if mention.AuthorUsername.Valid {
		author = mention.AuthorUsername.String
	}
	w.mon.NotifyMentions(ctx, payload.WorkspaceID, []monitor.SignalAlert{{
		ID:       mention.ID,
		Platform: string(mention.Platform),
		Title:    title,
		URL:      mention.Url,
		Author:   author,
		Content:  mention.Content,
	}})

	if err := markProcessed(ctx, w.cfg.q, group, w.cfg.consumerName, env.EventID, stream, msg.ID, payload.WorkspaceID); err != nil {
		return err
	}
	return nil
}

func (w *MentionWorkers) handleQualifierMessage(ctx context.Context, stream, group string, msg goredis.XMessage) error {
	env, err := streamredis.MessageToEnvelope(msg)
	if err != nil {
		return err
	}
	if env.EventType != events.EventTypeMentionScored {
		return nil
	}
	already, err := processedOrSkip(ctx, w.cfg.q, group, env.EventID)
	if err != nil {
		return err
	}
	if already {
		return nil
	}

	var payload events.MentionScoredPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return err
	}
	w.mon.QualifyMentionFromScore(ctx, payload)

	if err := markProcessed(ctx, w.cfg.q, group, w.cfg.consumerName, env.EventID, stream, msg.ID, payload.WorkspaceID); err != nil {
		return err
	}
	return nil
}

// Handler returns the message handler for retry autoclaim by group.
func (w *MentionWorkers) Handler(group string) messageHandler {
	switch group {
	case events.GroupMentionScorers:
		return w.handleScorerMessage
	case events.GroupMentionNotifiers:
		return w.handleNotifierMessage
	case events.GroupMentionQualifiers:
		return w.handleQualifierMessage
	default:
		return nil
	}
}
