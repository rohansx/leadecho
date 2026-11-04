package onboarding

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

const maxAnalysisText = 8000

// AnalysisText joins crawled pages for the product analysis prompt: the
// homepage first, then the other pages under their titles, capped.
func AnalysisText(pages []Page) string {
	var b strings.Builder
	for i, p := range pages {
		if i > 0 {
			fmt.Fprintf(&b, "\n\n## %s (%s)\n", p.Title, p.URL)
		}
		b.WriteString(p.Text)
		if b.Len() >= maxAnalysisText {
			break
		}
	}
	return capBytes(b.String(), maxAnalysisText)
}

// HeuristicAnalysis is the keyless fallback when no LLM is configured: the
// product name from the homepage title (or domain) and its first sentence.
// The user fills in the rest on the brand profile card.
func HeuristicAnalysis(pages []Page) *ai.ProductAnalysis {
	if len(pages) == 0 {
		return &ai.ProductAnalysis{}
	}
	home := pages[0]
	name := strings.TrimSpace(home.Title)
	if i := strings.IndexAny(name, "-|:–—·"); i >= 0 {
		name = strings.TrimSpace(name[:i])
	}
	if name == "" || len(strings.Fields(name)) > 3 {
		name = titleCase(strings.SplitN(hostOf(home.URL), ".", 2)[0])
	}
	desc := strings.TrimSpace(home.Text)
	if i := strings.IndexAny(desc, ".!?"); i > 0 {
		desc = desc[:i+1]
	}
	return &ai.ProductAnalysis{
		ProductName:        name,
		Description:        capLen(desc, 300),
		SuggestedPlatforms: []string{"reddit", "hackernews", "devto"},
		KeywordGroups:      ai.KeywordGroups{Brand: []string{name}},
	}
}

func capBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

// CalibrationExample is a user-labelled sample post for one listener.
type CalibrationExample struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
	Text     string `json:"text"`
	Relevant bool   `json:"relevant"` // 👍 = true, 👎 = false
}

// FeedbackStore is the query the calibration lookup needs (*database.Queries).
type FeedbackStore interface {
	ListSampleFeedbackForProfile(ctx context.Context, arg database.ListSampleFeedbackForProfileParams) ([]database.OnboardingSampleFeedback, error)
}

// ListCalibrationExamples returns the onboarding 👍/👎 examples for a
// listener (monitoring profile), newest first. The scorer reads them
// through CalibrationSource.
func ListCalibrationExamples(ctx context.Context, store FeedbackStore, workspaceID, profileID string) ([]CalibrationExample, error) {
	var pid pgtype.UUID
	if err := pid.Scan(profileID); err != nil {
		return nil, fmt.Errorf("invalid profile id: %w", err)
	}
	rows, err := store.ListSampleFeedbackForProfile(ctx, database.ListSampleFeedbackForProfileParams{WorkspaceID: workspaceID, ProfileID: pid})
	if err != nil {
		return nil, fmt.Errorf("list calibration examples: %w", err)
	}
	out := make([]CalibrationExample, 0, len(rows))
	for _, r := range rows {
		out = append(out, CalibrationExample{Platform: r.Platform, URL: r.Url, Text: r.Text, Relevant: r.Label == "up"})
	}
	return out, nil
}

// CalibrationSource feeds onboarding 👍/👎 samples to the scorer as few-shot
// examples (satisfies monitor.ExampleSource).
type CalibrationSource struct{ Store FeedbackStore }

func (c CalibrationSource) ScoringExamples(ctx context.Context, workspaceID, profileID string, limit int) ([]ai.ScoringExample, error) {
	if profileID == "" { // onboarding samples are always tied to a listener
		return nil, nil
	}
	rows, err := ListCalibrationExamples(ctx, c.Store, workspaceID, profileID)
	if err != nil {
		return nil, err
	}
	out := make([]ai.ScoringExample, 0, min(limit, len(rows)))
	for _, r := range rows {
		if len(out) >= limit {
			break
		}
		label := "down"
		if r.Relevant {
			label = "up"
		}
		out = append(out, ai.ScoringExample{Content: r.Text, Label: label, Note: "onboarding sample (" + r.Platform + ")"})
	}
	return out, nil
}
