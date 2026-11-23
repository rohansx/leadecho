//go:build live

package ai

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// go test -tags live -run TestLiveDossier -v ./internal/ai/   (ANTHROPIC_API_KEY)
func TestLiveDossier(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	in := DossierInput{
		Product:  "LeadEcho. Finds people describing a problem your product solves on Reddit, HN and Dev.to, and drafts replies.",
		Audience: "Founders and growth leads at B2B SaaS startups",
		Handle:   "iepathos", Platform: "hackernews",
		Identity: "Name: Glen Baker\nRole: Staff Software Engineer\nSummary: Staff engineer, ~14 years at startups; builds open-source Rust devtools (Debtmap, Stillwater).\n" +
			"github: https://github.com/iepathos [0.85]\nwebsite: https://entropicdrift.com [0.90]\nlinkedin: https://www.linkedin.com/in/glenbbaker [0.90]\nemail: mailto:iepathos@gmail.com [0.90]\n",
		Channels: []string{"email", "github", "linkedin"},
		Posts: []DossierPost{{ID: "m1", Platform: "hackernews", Date: "2026-11-20", Intent: "recommendation_ask",
			Title:   "Ask HN: How do you find your first users for an open-source dev tool?",
			Excerpt: "I've shipped Debtmap, a Rust code-debt analyzer. Posting on Reddit got a few stars but no real users. How did you find people who actually have the problem?"}},
	}
	start := time.Now()
	d, err := BuildPersonDossier(context.Background(), Provider{Name: "anthropic", APIKey: key, Model: "claude-sonnet-5-5"}, in)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(d, "", "  ")
	t.Logf("%s\n%s", time.Since(start).Round(time.Millisecond), b)
	if len(d.Pains)+len(d.BuyingSignals) == 0 {
		t.Error("expected at least one cited pain or buying signal from the post")
	}
}
