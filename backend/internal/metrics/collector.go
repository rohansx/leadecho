package metrics

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/events"
	streamredis "leadecho/internal/events/redis"
)

type Collector struct {
	streams  *streamredis.Client
	q        *database.Queries
	logger   zerolog.Logger
	interval time.Duration
}

func NewCollector(streams *streamredis.Client, q *database.Queries, logger zerolog.Logger, interval time.Duration) *Collector {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Collector{
		streams:  streams,
		q:        q,
		logger:   logger.With().Str("component", "metrics_collector").Logger(),
		interval: interval,
	}
}

func (c *Collector) Run(ctx context.Context) {
	Register()
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.collect(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collect(ctx)
		}
	}
}

func (c *Collector) collect(ctx context.Context) {
	for _, sg := range events.ProductionStreamGroups() {
		pending, err := c.streams.Pending(ctx, sg.Stream, sg.Group)
		if err != nil {
			c.logger.Debug().Err(err).Str("stream", sg.Stream).Str("group", sg.Group).Msg("pending metrics")
			continue
		}
		SetPending(sg.Stream, sg.Group, float64(pending.Count))
	}

	count, err := c.q.CountOpenDeadLetterEvents(ctx)
	if err != nil {
		c.logger.Debug().Err(err).Msg("dlq metrics")
		return
	}
	SetDLQOpen(float64(count))
}
