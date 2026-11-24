package monitor

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

	"leadecho/internal/ai"
)

func TestAutoDraftable(t *testing.T) {
	for _, c := range []struct {
		name string
		r    *ai.ClassifyResult
		want bool
	}{
		{"strong buyer lead", &ai.ClassifyResult{RelevanceScore: 8.5, Intent: "recommendation_ask", AuthorRole: "buyer"}, true},
		{"exactly 7", &ai.ClassifyResult{RelevanceScore: 7, Intent: "complaint", AuthorRole: "buyer"}, true},
		{"a lead, but under 7", &ai.ClassifyResult{RelevanceScore: 6.8, Intent: "complaint", AuthorRole: "buyer"}, false},
		{"high score, not a Leads-tab intent", &ai.ClassifyResult{RelevanceScore: 9, Intent: "general", AuthorRole: "buyer"}, false},
		{"non-buyer", &ai.ClassifyResult{RelevanceScore: 9, Intent: "buy_signal", AuthorRole: "non_buyer"}, false},
		{"role unknown", &ai.ClassifyResult{RelevanceScore: 9, Intent: "buy_signal", AuthorRole: "unknown"}, false},
		{"no result", nil, false},
	} {
		if got := autoDraftable(c.r, leadScoreThreshold); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

type countingDrafter struct{ calls []string }

func (c *countingDrafter) AutoDraft(_ context.Context, _, mentionID string) (bool, error) {
	c.calls = append(c.calls, mentionID)
	return true, nil
}

func TestAutoDraftOnlyAsksForBuyerLeads(t *testing.T) {
	d := &countingDrafter{}
	m := &Monitor{logger: zerolog.Nop()}
	lead := &ai.ClassifyResult{RelevanceScore: 8.5, Intent: "recommendation_ask", AuthorRole: "buyer"}

	const min = leadScoreThreshold
	m.autoDraft(context.Background(), mentionAlert{ID: "off"}, lead, min) // no drafter wired: nothing happens
	m.SetAutoDrafter(d)
	m.autoDraft(context.Background(), mentionAlert{ID: "lead"}, lead, min)
	m.autoDraft(context.Background(), mentionAlert{ID: "candidate"}, &ai.ClassifyResult{RelevanceScore: 9, Intent: "buy_signal", AuthorRole: "non_buyer"}, min)
	m.autoDraft(context.Background(), mentionAlert{ID: "weak"}, &ai.ClassifyResult{RelevanceScore: 6, Intent: "complaint", AuthorRole: "buyer"}, min)
	m.autoDraft(context.Background(), mentionAlert{ID: "under strict"}, &ai.ClassifyResult{RelevanceScore: 7.2, Intent: "complaint", AuthorRole: "buyer"}, 7.5)

	if len(d.calls) != 1 || d.calls[0] != "lead" {
		t.Fatalf("drafted for %v, want only the buyer lead", d.calls)
	}
}
