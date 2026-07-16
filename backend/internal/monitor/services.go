package monitor

import (
	"context"

	"leadecho/internal/ai"
	"leadecho/internal/database"
	"leadecho/internal/events"
)

func (m *Monitor) ScoreMentionBatch(ctx context.Context, wsID string, alerts []SignalAlert) {
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
	m.batchScoreMentions(ctx, wsID, ma)
}

func (m *Monitor) NotifyMentions(ctx context.Context, wsID string, alerts []SignalAlert) {
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
	m.notifyNewMentions(ctx, wsID, ma)
}

// QualifyMentionFromScore runs lead qualification from a mention.scored event payload.
func (m *Monitor) QualifyMentionFromScore(ctx context.Context, payload events.MentionScoredPayload) {
	if payload.RelevanceScore < 7.0 {
		return
	}
	intent := database.IntentType(payload.Intent)
	if intent != database.IntentTypeBuySignal &&
		intent != database.IntentTypeRecommendationAsk &&
		intent != database.IntentTypeComplaint {
		return
	}

	alert := mentionAlert{
		ID:          payload.MentionID,
		WorkspaceID: payload.WorkspaceID,
		Platform:    payload.Platform,
		Title:       payload.Title,
		URL:         payload.URL,
		Author:      payload.Author,
		Content:     payload.Content,
	}
	result := &ai.ClassifyResult{
		Intent:         payload.Intent,
		RelevanceScore: float64(payload.RelevanceScore),
	}
	m.qualifyAsLead(ctx, alert, result)
}
