//go:build live

// Live check of the funding-signal prompts: the screen keeps a funded
// software company and drops one that would not hire engineers, and the
// first message opens on the round without inventing a job. Spends about a
// cent. Run:
//
//	ANTHROPIC_API_KEY=... go test -tags live -run TestFundingLive -v ./internal/ai/
package ai

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFundingLive(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	p := Provider{Name: "anthropic", APIKey: key, Model: "claude-sonnet-5-5", Effort: "low"}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	product, overview := "Utkrusht", "A technical skill assessment platform for hiring software engineers: candidates fix real bugs in real codebases instead of solving puzzles."
	audience := "Engineering leaders and technical recruiters at tech companies of 20 to 5,000 people that are hiring developers"

	fits, err := ScreenHiringJobs(ctx, p, HiringScreen{Product: product, Overview: overview, Audience: audience,
		Roles: []string{"software engineer", "backend engineer"}, MaxCompanySize: 5000,
		Jobs: []HiringJob{
			{Company: "Northpane", Funding: "a Seed round of $6 million, announced 2026-11-13", Website: "northpane.dev",
				SourceURL: "https://news.example/northpane-raises-6m-seed-for-its-observability-platform-for-ai-agents"},
			{Company: "Verdant Oncology", Funding: "a Series A round of $40 million, announced 2026-11-12", Website: "verdantoncology.example",
				SourceURL: "https://news.example/verdant-oncology-closes-40m-series-a-to-advance-phase-2-trial"},
			{Company: "Loomfield", Title: "Senior Backend Engineer", Location: "Austin, TX"},
			{Company: "Halden Staffing Partners", Title: "Software Engineer (contract, for our client)", Location: "Remote"},
		}})
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	kept := map[int]string{}
	for _, f := range fits {
		kept[f.Index] = f.Reason
	}
	t.Logf("screen kept: %v", kept)
	if _, ok := kept[0]; !ok {
		t.Error("a funded developer-tools company should be kept")
	}
	if _, ok := kept[1]; ok {
		t.Error("a funded clinical-stage biotech should be dropped")
	}
	if _, ok := kept[2]; !ok {
		t.Error("the plain posting should still be kept")
	}
	if _, ok := kept[3]; ok {
		t.Error("the staffing firm should still be dropped")
	}

	msg, err := DraftOutreach(ctx, p, Outreach{Product: product, Overview: overview, Positioning: "Real debugging tasks, graded automatically, instead of algorithm puzzles.",
		ContactName: "Dana Whitlock", ContactTitle: "Co-Founder & CTO", Company: "Northpane", Funding: "a Seed round of $6 million, announced 2026-11-13"})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	t.Logf("funding draft:\n%s", msg)
	_, body, _ := strings.Cut(msg, "\n\n")
	if !strings.HasPrefix(msg, "Subject: ") || !strings.Contains(body, "Dana") {
		t.Error("the draft should have a subject line and greet by first name")
	}
	first, _, _ := strings.Cut(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(body), "Hi Dana,")), "\n\n")
	if !strings.Contains(strings.ToLower(first), "seed") || !strings.Contains(first, "6") {
		t.Errorf("the opening should state the round: %q", first)
	}
	for _, invented := range []string{"investor", "valuation", "led by", "congrat", "job post", "you posted", "open role"} {
		if strings.Contains(strings.ToLower(msg), invented) {
			t.Errorf("the draft says %q, which is not in the stored fields", invented)
		}
	}
}
