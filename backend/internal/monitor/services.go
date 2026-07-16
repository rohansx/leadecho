package monitor

import "context"

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
