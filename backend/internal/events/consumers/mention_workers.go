package consumers

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/events"
	streamredis "leadecho/internal/events/redis"
	"leadecho/internal/monitor"
)

type MentionWorkers struct {
	q            *database.Queries
	streams      *streamredis.Client
	logger       zerolog.Logger
	mon          *monitor.Monitor
	consumerName string
	batchSize    int64
	block        time.Duration
	claimIdle    time.Duration
	maxAttempts  int64
}

func NewMentionWorkers(q *database.Queries, streams *streamredis.Client, mon *monitor.Monitor, logger zerolog.Logger, consumerName string, batchSize, blockMS, claimIdleMS, maxAttempts int64) *MentionWorkers {
	if batchSize <= 0 {
		batchSize = 50
	}
	if blockMS <= 0 {
		blockMS = 5000
	}
	if claimIdleMS <= 0 {
		claimIdleMS = 120000
	}
	if maxAttempts <= 0 {
		maxAttempts = 8
	}
	return &MentionWorkers{
		q:            q,
		streams:      streams,
		logger:       logger.With().Str("component", "mention_workers").Logger(),
		mon:          mon,
		consumerName: consumerName,
		batchSize:    batchSize,
		block:        time.Duration(blockMS) * time.Millisecond,
		claimIdle:    time.Duration(claimIdleMS) * time.Millisecond,
		maxAttempts:  maxAttempts,
	}
}

func (w *MentionWorkers) StartScorer(ctx context.Context) error {
	if err := w.streams.EnsureGroup(ctx, events.StreamMentionEvents, events.GroupMentionScorers); err != nil {
		return err
	}
	return w.loop(ctx, events.StreamMentionEvents, events.GroupMentionScorers, w.handleScorerMessage)
}

func (w *MentionWorkers) StartNotifier(ctx context.Context) error {
	if err := w.streams.EnsureGroup(ctx, events.StreamMentionEvents, events.GroupMentionNotifiers); err != nil {
		return err
	}
	return w.loop(ctx, events.StreamMentionEvents, events.GroupMentionNotifiers, w.handleNotifierMessage)
}

func (w *MentionWorkers) StartRetryManager(ctx context.Context) error {
	ticker := time.NewTicker(w.claimIdle)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			for _, group := range []string{events.GroupMentionScorers, events.GroupMentionNotifiers} {
				start := "0-0"
				msgs, next, err := w.streams.AutoClaim(ctx, events.StreamMentionEvents, group, w.consumerName, w.claimIdle, start, w.batchSize)
				if err != nil {
					w.logger.Error().Err(err).Str("group", group).Msg("streams: autoclaim failed")
					continue
				}
				start = next
				for _, msg := range msgs {
					var handleErr error
					switch group {
					case events.GroupMentionScorers:
						handleErr = w.handleScorerMessage(ctx, events.StreamMentionEvents, group, msg)
					case events.GroupMentionNotifiers:
						handleErr = w.handleNotifierMessage(ctx, events.StreamMentionEvents, group, msg)
					}
					if handleErr == nil {
						_ = w.streams.Ack(ctx, events.StreamMentionEvents, group, msg.ID)
					} else {
						w.handleConsumeError(ctx, events.StreamMentionEvents, group, msg, handleErr)
					}
					_, _ = w.q.UpsertConsumerCheckpoint(ctx, database.UpsertConsumerCheckpointParams{
						StreamName:         events.StreamMentionEvents,
						ConsumerGroup:      group,
						ConsumerName:       w.consumerName,
						LastRedisMessageID: textOrNull(msg.ID),
						PendingCount:       0,
						LastError:          textOrNull(errorString(handleErr)),
						HeartbeatAt:        time.Now().UTC(),
					})
				}
				_ = start
			}
		}
	}
}

func (w *MentionWorkers) loop(ctx context.Context, stream, group string, handler func(context.Context, string, string, goredis.XMessage) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msgs, err := w.streams.ReadGroup(ctx, stream, group, w.consumerName, w.batchSize, w.block)
		if err != nil {
			w.logger.Error().Err(err).Str("stream", stream).Str("group", group).Msg("streams: read failed")
			time.Sleep(time.Second)
			continue
		}
		if len(msgs) == 0 {
			continue
		}

		pending := int64(len(msgs))
		lastID := ""
		for _, msg := range msgs {
			lastID = msg.ID
			err := handler(ctx, stream, group, msg)
			if err == nil {
				_ = w.streams.Ack(ctx, stream, group, msg.ID)
				continue
			}
			w.handleConsumeError(ctx, stream, group, msg, err)
		}
		_, _ = w.q.UpsertConsumerCheckpoint(ctx, database.UpsertConsumerCheckpointParams{
			StreamName:         stream,
			ConsumerGroup:      group,
			ConsumerName:       w.consumerName,
			LastRedisMessageID: textOrNull(lastID),
			PendingCount:       pending,
			LastError:          textOrNull(""),
			HeartbeatAt:        time.Now().UTC(),
		})
	}
}

func (w *MentionWorkers) handleScorerMessage(ctx context.Context, stream, group string, msg goredis.XMessage) error {
	env, err := streamredis.MessageToEnvelope(msg)
	if err != nil {
		return err
	}
	if env.EventType != events.EventTypeMentionIngested {
		return nil
	}
	already, err := w.q.HasConsumerProcessedEvent(ctx, database.HasConsumerProcessedEventParams{
		ConsumerGroup: group,
		EventID:       env.EventID,
	})
	if err == nil && already {
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

	_, err = w.q.CreateConsumerProcessedEvent(ctx, database.CreateConsumerProcessedEventParams{
		ConsumerGroup:  group,
		ConsumerName:   w.consumerName,
		EventID:        env.EventID,
		StreamName:     stream,
		RedisMessageID: msg.ID,
		WorkspaceID:    uuidOrNull(payload.WorkspaceID),
		ResultStatus:   "processed",
	})
	if err != nil && !isDuplicate(err) {
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
	already, err := w.q.HasConsumerProcessedEvent(ctx, database.HasConsumerProcessedEventParams{
		ConsumerGroup: group,
		EventID:       env.EventID,
	})
	if err == nil && already {
		return nil
	}

	var payload events.MentionNotificationRequestedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return err
	}
	mention, err := w.q.GetMention(ctx, database.GetMentionParams{ID: payload.MentionID, WorkspaceID: payload.WorkspaceID})
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

	_, err = w.q.CreateConsumerProcessedEvent(ctx, database.CreateConsumerProcessedEventParams{
		ConsumerGroup:  group,
		ConsumerName:   w.consumerName,
		EventID:        env.EventID,
		StreamName:     stream,
		RedisMessageID: msg.ID,
		WorkspaceID:    uuidOrNull(payload.WorkspaceID),
		ResultStatus:   "processed",
	})
	if err != nil && !isDuplicate(err) {
		return err
	}
	return nil
}

func (w *MentionWorkers) handleConsumeError(ctx context.Context, stream, group string, msg goredis.XMessage, err error) {
	env, parseErr := streamredis.MessageToEnvelope(msg)
	if parseErr != nil {
		env.EventID = ""
		env.WorkspaceID = ""
		env.Payload = mustMarshal(msg.Values)
	}
	_, _ = w.q.CreateDeadLetterEvent(ctx, database.CreateDeadLetterEventParams{
		EventID:          env.EventID,
		StreamName:       stream,
		ConsumerGroup:    group,
		ConsumerName:     w.consumerName,
		WorkspaceID:      uuidOrNull(env.WorkspaceID),
		RedisMessageID:   textOrNull(msg.ID),
		Payload:          env.Payload,
		ErrorCode:        textOrNull("consumer_error"),
		ErrorClass:       classifyError(err),
		ErrorMessage:     err.Error(),
		StackExcerpt:     textOrNull(""),
		RetryCount:       1,
		FirstSeenAt:      time.Now().UTC(),
		LastSeenAt:       time.Now().UTC(),
		ResolutionStatus: "open",
	})
	_, _ = w.q.UpsertConsumerCheckpoint(ctx, database.UpsertConsumerCheckpointParams{
		StreamName:         stream,
		ConsumerGroup:      group,
		ConsumerName:       w.consumerName,
		LastRedisMessageID: textOrNull(msg.ID),
		PendingCount:       1,
		LastError:          textOrNull(err.Error()),
		HeartbeatAt:        time.Now().UTC(),
	})
}

func classifyError(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return "transient"
	}
}

func isDuplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "duplicate key") || contains(msg, "unique constraint")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func textOrNull(v string) pgtype.Text {
	return pgtype.Text{String: v, Valid: v != ""}
}

func uuidOrNull(v string) pgtype.UUID {
	var u pgtype.UUID
	if v == "" {
		return u
	}
	_ = u.Scan(v)
	return u
}
