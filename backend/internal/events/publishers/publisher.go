package publishers

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/events"
	streamredis "leadecho/internal/events/redis"
)

type Publisher struct {
	q       *database.Queries
	streams *streamredis.Client
	logger  zerolog.Logger
}

func New(q *database.Queries, streams *streamredis.Client, logger zerolog.Logger) *Publisher {
	return &Publisher{
		q:       q,
		streams: streams,
		logger:  logger.With().Str("component", "event_publisher").Logger(),
	}
}

func (p *Publisher) Publish(ctx context.Context, env events.Envelope) (string, error) {
	if err := env.Validate(); err != nil {
		return "", err
	}

	stream := events.StreamForEventType(env.EventType)
	eventLog, err := p.q.CreateEventLog(ctx, database.CreateEventLogParams{
		EventID:         env.EventID,
		StreamName:      stream,
		EventType:       env.EventType,
		SchemaVersion:   env.SchemaVersion,
		WorkspaceID:     uuidOrNull(env.WorkspaceID),
		AggregateType:   env.AggregateType,
		AggregateID:     env.AggregateID,
		Producer:        env.Producer,
		Payload:         env.Payload,
		IdempotencyKey:  env.IdempotencyKey,
		CorrelationID:   textOrNull(env.CorrelationID),
		CausationID:     textOrNull(env.CausationID),
		TraceID:         textOrNull(env.TraceID),
		OccurredAt:      env.OccurredAt,
		ReplayOfEventID: uuidOrNull(""),
	})
	if err != nil {
		return "", fmt.Errorf("create event log: %w", err)
	}

	msgID, err := p.streams.Publish(ctx, stream, env)
	if err != nil {
		_, markErr := p.q.MarkEventLogPublishFailed(ctx, database.MarkEventLogPublishFailedParams{
			EventID:       eventLog.EventID,
			PublishStatus: "failed",
			LastError:     textOrNull(err.Error()),
		})
		if markErr != nil {
			p.logger.Error().Err(markErr).Str("event_id", env.EventID).Msg("mark publish failure")
		}
		return "", err
	}

	if _, err := p.q.MarkEventLogPublished(ctx, database.MarkEventLogPublishedParams{
		EventID:        eventLog.EventID,
		RedisMessageID: textOrNull(msgID),
		PublishStatus:  "published",
		LastError:      textOrNull(""),
	}); err != nil {
		return "", fmt.Errorf("mark event published: %w", err)
	}
	return msgID, nil
}

func textOrNull(v string) pgtype.Text {
	return pgtype.Text{String: v, Valid: v != ""}
}

func uuidOrNull(v string) pgtype.UUID {
	if v == "" {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: uuidBytes(v), Valid: true}
}

func uuidBytes(s string) [16]byte {
	var out [16]byte
	j := 0
	for i := 0; i < len(s) && j < 16; i++ {
		if s[i] == '-' {
			continue
		}
		if i+1 >= len(s) {
			break
		}
		hi := fromHex(s[i])
		lo := fromHex(s[i+1])
		if hi < 0 || lo < 0 {
			break
		}
		out[j] = byte(hi<<4 | lo)
		j++
		i++
	}
	return out
}

func fromHex(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10
	default:
		return -1
	}
}
