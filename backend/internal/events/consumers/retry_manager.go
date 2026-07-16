package consumers

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/events"
	"leadecho/internal/metrics"
	streamredis "leadecho/internal/events/redis"
)

type RetryManager struct {
	cfg      streamConfig
	handlers map[string]map[string]messageHandler
}

func NewRetryManager(q *database.Queries, streams *streamredis.Client, logger zerolog.Logger, consumerName string, batchSize, blockMS, claimIdleMS, maxAttempts int64) *RetryManager {
	return &RetryManager{
		cfg:      newStreamConfig(q, streams, logger.With().Str("component", "retry_manager").Logger(), consumerName, batchSize, blockMS, claimIdleMS, maxAttempts),
		handlers: make(map[string]map[string]messageHandler),
	}
}

func (r *RetryManager) Register(stream, group string, handler messageHandler) {
	if r.handlers[stream] == nil {
		r.handlers[stream] = make(map[string]messageHandler)
	}
	r.handlers[stream][group] = handler
}

func (r *RetryManager) Start(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.claimIdle)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			for stream, groups := range r.handlers {
				for group, handler := range groups {
					start := "0-0"
					msgs, next, err := r.cfg.streams.AutoClaim(ctx, stream, group, r.cfg.consumerName, r.cfg.claimIdle, start, r.cfg.batchSize)
					if err != nil {
						r.cfg.logger.Error().Err(err).Str("stream", stream).Str("group", group).Msg("streams: autoclaim failed")
						continue
					}
					start = next
					for _, msg := range msgs {
						started := time.Now()
						handleErr := handler(ctx, stream, group, msg)
						if handleErr == nil {
							metrics.ObserveProcessed(stream, group, "success", time.Since(started).Seconds())
							_ = r.cfg.streams.Ack(ctx, stream, group, msg.ID)
						} else {
							metrics.ObserveProcessed(stream, group, "error", time.Since(started).Seconds())
							r.cfg.handleConsumeError(ctx, stream, group, msg, handleErr)
						}
						_, _ = r.cfg.q.UpsertConsumerCheckpoint(ctx, database.UpsertConsumerCheckpointParams{
							StreamName:         stream,
							ConsumerGroup:      group,
							ConsumerName:       r.cfg.consumerName,
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
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// EnsureAllGroups creates consumer groups that may not exist yet.
func EnsureAllGroups(ctx context.Context, streams *streamredis.Client) error {
	for _, sg := range events.ProductionStreamGroups() {
		if err := streams.EnsureGroup(ctx, sg.Stream, sg.Group); err != nil {
			return err
		}
	}
	return nil
}