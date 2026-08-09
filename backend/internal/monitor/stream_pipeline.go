package monitor

import (
	"context"
	"fmt"

	"leadecho/internal/events"
)

func (m *Monitor) handleNewMentionBatch(ctx context.Context, wsID string, alerts []mentionAlert, producer string) {
	if len(alerts) > 0 {
		if m.streamsEnabled && m.eventPublisher != nil {
			m.publishMentionIngestedBatch(ctx, wsID, alerts, producer)
			if !m.streamsDualWrite && !m.inlineFallback {
				return
			}
		}

		if m.inlineFallback || !m.streamsEnabled || m.streamsDualWrite {
			m.batchScoreMentions(ctx, wsID, alerts)
			m.notifyNewMentions(ctx, wsID, alerts)
		}
	}

	// Always backfill unclassified mentions from earlier ticks, even if this
	// crawl produced no new alerts — e.g. when an AI provider was just configured
	// and existing mentions need to be retroactively scored.
	if m.inlineFallback || !m.streamsEnabled || m.streamsDualWrite {
		m.backfillUnclassified(ctx, wsID)
	}
}

func (m *Monitor) publishMentionIngestedBatch(ctx context.Context, wsID string, alerts []mentionAlert, producer string) {
	for _, alert := range alerts {
		env, err := events.NewEnvelope(
			events.EventTypeMentionIngested,
			events.AggregateTypeMention,
			alert.ID,
			producer,
			wsID,
			fmt.Sprintf("%s:%s", events.EventTypeMentionIngested, alert.ID),
			events.MentionIngestedPayload{
				MentionID:   alert.ID,
				WorkspaceID: wsID,
				Platform:    alert.Platform,
				Title:       alert.Title,
				URL:         alert.URL,
				Author:      alert.Author,
				Content:     alert.Content,
				Keyword:     alert.Keyword,
				Source:      producer,
			},
		)
		if err != nil {
			m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: build mention.ingested")
			continue
		}
		if _, err := m.eventPublisher.Publish(ctx, env); err != nil {
			m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("streams: publish mention.ingested")
		}
	}
}

func (m *Monitor) ProcessIngestedSignals(ctx context.Context, wsID string, alerts []SignalAlert) {
	ma := make([]mentionAlert, len(alerts))
	for i, a := range alerts {
		ma[i] = mentionAlert{
			ID:          a.ID,
			WorkspaceID: wsID,
			Platform:    a.Platform,
			Title:       a.Title,
			URL:         a.URL,
			Author:      a.Author,
			Content:     a.Content,
		}
	}
	m.handleNewMentionBatch(ctx, wsID, ma, "extension")
}
