package monitor

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

// scoring: per-listener scoring context (Looks for / Skips + few-shot examples).

// ExampleSource supplies labelled few-shot examples for a listener
// (profileID "" = mentions with no listener). Inbox 👍/👎 feedback is the
// built-in source; onboarding calibration can plug in via AddExampleSource.
type ExampleSource interface {
	ScoringExamples(ctx context.Context, workspaceID, profileID string, limit int) ([]ai.ScoringExample, error)
}

// AddExampleSource registers an extra few-shot source. Examples from
// earlier sources (inbox feedback first) win when the cap is reached.
// Call during setup, before Run.
func (m *Monitor) AddExampleSource(s ExampleSource) { m.extraExamples = append(m.extraExamples, s) }

// FeedbackExamples reads the latest inbox 👍/👎 labels for a listener.
type FeedbackExamples struct{ Q *database.Queries }

func (f FeedbackExamples) ScoringExamples(ctx context.Context, workspaceID, profileID string, limit int) ([]ai.ScoringExample, error) {
	rows, err := f.Q.ListScoringExamples(ctx, database.ListScoringExamplesParams{
		WorkspaceID: workspaceID,
		ProfileID:   pgUUID(profileID), // "" → NULL → workspace-level examples
		Lim:         int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ai.ScoringExample, len(rows))
	for i, r := range rows {
		out[i] = ai.ScoringExample{Title: r.Title, Content: r.Content, Label: r.Label, Note: r.Note}
	}
	return out, nil
}

// scoringContextFor builds the classification context for one mention.
func (m *Monitor) scoringContextFor(ctx context.Context, wsID, mentionID string) ai.ScoringContext {
	return ScoringContextFor(ctx, m.q, m.extraExamples, wsID, mentionID, func(err error) {
		m.logger.Warn().Err(err).Str("mention_id", mentionID).Msg("scorer: load scoring context")
	})
}

// ScoringContextFor builds a mention's scoring context: its listener's
// product, Looks for / Skips, and few-shot examples (inbox feedback first,
// then extra sources). Lookup failures are reported to onErr and degrade
// to generic scoring rather than blocking it.
func ScoringContextFor(ctx context.Context, q *database.Queries, extra []ExampleSource, wsID, mentionID string, onErr func(error)) ai.ScoringContext {
	var prof *database.GetMentionScoringProfileRow
	row, err := q.GetMentionScoringProfile(ctx, database.GetMentionScoringProfileParams{MentionID: mentionID, WorkspaceID: wsID})
	switch {
	case err == nil:
		prof = &row
	case !errors.Is(err, pgx.ErrNoRows):
		onErr(err)
	}
	profileID := ""
	if prof != nil {
		profileID = prof.ID
	}
	sources := append([]ExampleSource{FeedbackExamples{Q: q}}, extra...)
	return buildScoringContext(prof, collectExamples(ctx, sources, wsID, profileID, ai.MaxScoringExamples, onErr))
}

// collectExamples merges sources in order up to limit, skipping failures
// and examples without a valid label.
func collectExamples(ctx context.Context, sources []ExampleSource, wsID, profileID string, limit int, onErr func(error)) []ai.ScoringExample {
	var out []ai.ScoringExample
	for _, s := range sources {
		if len(out) >= limit {
			break
		}
		ex, err := s.ScoringExamples(ctx, wsID, profileID, limit-len(out))
		if err != nil {
			onErr(err)
			continue
		}
		for _, e := range ex {
			if len(out) < limit && (e.Label == "up" || e.Label == "down") {
				out = append(out, e)
			}
		}
	}
	return out
}

func buildScoringContext(prof *database.GetMentionScoringProfileRow, examples []ai.ScoringExample) ai.ScoringContext {
	sc := ai.ScoringContext{Examples: examples}
	if prof == nil {
		return sc
	}
	sc.Product = "- " + prof.Name + ": " + prof.Description
	sc.LooksFor = strings.TrimSpace(prof.LooksFor)
	sc.Skips = strings.TrimSpace(prof.Skips)
	return sc
}

// storeIntentTags persists the taxonomy tags + sentiment next to the legacy intent.
func (m *Monitor) storeIntentTags(ctx context.Context, wsID, mentionID string, res *ai.ClassifyResult) {
	tags := res.IntentTags
	if tags == nil {
		tags = []string{} // column is NOT NULL
	}
	if err := m.q.UpdateMentionTags(ctx, database.UpdateMentionTagsParams{
		IntentTags:  tags,
		Sentiment:   pgtype.Text{String: res.Sentiment, Valid: res.Sentiment != ""},
		ID:          mentionID,
		WorkspaceID: wsID,
	}); err != nil {
		m.logger.Error().Err(err).Str("mention_id", mentionID).Msg("scorer: failed to store intent tags")
	}
}
