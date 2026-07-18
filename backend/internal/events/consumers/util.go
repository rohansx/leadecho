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
	"leadecho/internal/metrics"
	streamredis "leadecho/internal/events/redis"
)

type streamConfig struct {
	q            *database.Queries
	streams      *streamredis.Client
	logger       zerolog.Logger
	consumerName string
	batchSize    int64
	block        time.Duration
	claimIdle    time.Duration
	maxAttempts  int64
}

func newStreamConfig(q *database.Queries, streams *streamredis.Client, logger zerolog.Logger, consumerName string, batchSize, blockMS, claimIdleMS, maxAttempts int64) streamConfig {
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
	return streamConfig{
		q:            q,
		streams:      streams,
		logger:       logger,
		consumerName: consumerName,
		batchSize:    batchSize,
		block:        time.Duration(blockMS) * time.Millisecond,
		claimIdle:    time.Duration(claimIdleMS) * time.Millisecond,
		maxAttempts:  maxAttempts,
	}
}

type messageHandler func(context.Context, string, string, goredis.XMessage) error

func (c streamConfig) loop(ctx context.Context, stream, group string, handler messageHandler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msgs, err := c.streams.ReadGroup(ctx, stream, group, c.consumerName, c.batchSize, c.block)
		if err != nil {
			c.logger.Error().Err(err).Str("stream", stream).Str("group", group).Msg("streams: read failed")
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
			started := time.Now()
			err := handler(ctx, stream, group, msg)
			if err == nil {
				metrics.ObserveProcessed(stream, group, "success", time.Since(started).Seconds())
				_ = c.streams.Ack(ctx, stream, group, msg.ID)
				continue
			}
			metrics.ObserveProcessed(stream, group, "error", time.Since(started).Seconds())
			c.handleConsumeError(ctx, stream, group, msg, err)
		}
		_, _ = c.q.UpsertConsumerCheckpoint(ctx, database.UpsertConsumerCheckpointParams{
			StreamName:         stream,
			ConsumerGroup:      group,
			ConsumerName:       c.consumerName,
			LastRedisMessageID: textOrNull(lastID),
			PendingCount:       pending,
			LastError:          textOrNull(""),
			HeartbeatAt:        time.Now().UTC(),
		})
	}
}

func (c streamConfig) handleConsumeError(ctx context.Context, stream, group string, msg goredis.XMessage, err error) {
	env, parseErr := streamredis.MessageToEnvelope(msg)
	if parseErr != nil {
		env.EventID = ""
		env.WorkspaceID = ""
		env.Payload = mustMarshal(msg.Values)
	}
	_, _ = c.q.CreateDeadLetterEvent(ctx, database.CreateDeadLetterEventParams{
		EventID:          env.EventID,
		StreamName:       stream,
		ConsumerGroup:    group,
		ConsumerName:     c.consumerName,
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
	_, _ = c.q.UpsertConsumerCheckpoint(ctx, database.UpsertConsumerCheckpointParams{
		StreamName:         stream,
		ConsumerGroup:      group,
		ConsumerName:       c.consumerName,
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

func processedOrSkip(ctx context.Context, q *database.Queries, group, eventID string) (bool, error) {
	already, err := q.HasConsumerProcessedEvent(ctx, database.HasConsumerProcessedEventParams{
		ConsumerGroup: group,
		EventID:       eventID,
	})
	if err != nil {
		return false, err
	}
	return already, nil
}

func markProcessed(ctx context.Context, q *database.Queries, group, consumerName, eventID, stream, msgID, workspaceID string) error {
	_, err := q.CreateConsumerProcessedEvent(ctx, database.CreateConsumerProcessedEventParams{
		ConsumerGroup:  group,
		ConsumerName:   consumerName,
		EventID:        eventID,
		StreamName:     stream,
		RedisMessageID: msgID,
		WorkspaceID:    uuidOrNull(workspaceID),
		ResultStatus:   "processed",
	})
	if err != nil && !isDuplicate(err) {
		return err
	}
	return nil
}
