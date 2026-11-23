//go:build live

package identity

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/sandbox"
)

// Tier a, then the research agent (Claude Sonnet 5.5 + E2B), then the
// evidence check. Costs real money: ANTHROPIC_API_KEY and E2B_API_KEY.
//
//	go test -tags live -run TestLiveAgent -v -timeout 30m ./internal/identity/
//	IDENTITY_SEEDS_FILE=seeds.json overrides the seeds.
func TestLiveAgent(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	seeds := []Seed{{Platform: "hackernews", Handle: "patio11"}}
	if f := os.Getenv("IDENTITY_SEEDS_FILE"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &seeds); err != nil {
			t.Fatal(err)
		}
	}
	r := New(os.Getenv("GITHUB_TOKEN"), nil)
	sb := sandbox.New(os.Getenv("E2B_API_KEY"), "")
	p := ai.Provider{Name: "anthropic", APIKey: key, Model: ai.PersonResearchModel}
	for _, s := range seeds {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
		start := time.Now()
		prof, err := r.Resolve(ctx, s)
		if err != nil {
			prof = &Profile{Tier: "a"}
		}
		var known []string
		for _, l := range prof.Links {
			if l.Confidence >= 0.5 {
				known = append(known, l.Kind+" "+l.URL)
			}
		}
		var tools ai.SandboxTools
		sess := sb.Open()
		if sess != nil {
			tools = sess
		}
		f, u, err := ai.ResearchPerson(ctx, p, ai.PersonResearch{Platform: s.Platform, Handle: s.Handle, ProfileURL: s.ProfileURL, Name: prof.Name, Known: known}, tools)
		sess.Close()
		cancel()
		if err != nil {
			t.Logf("%s/%s: agent error after %s: %v", s.Platform, s.Handle, time.Since(start).Round(time.Second), err)
			continue
		}
		merged := MergeAgent(prof, f)
		if os.Getenv("IDENTITY_DEBUG") != "" {
			for u, text := range f.Seen {
				var ls []string
				for _, c := range ExtractLinks(text) {
					ls = append(ls, c.URL)
				}
				t.Logf("SEEN %s (%d chars) links=%v\n  head=%q", u, len(text), ls, clip(text, 400))
			}
			for _, l := range f.Links {
				t.Logf("CLAIM %s evidence=%s", l.URL, l.Evidence)
			}
		}
		claimed, _ := json.Marshal(f.Links)
		out, _ := json.MarshalIndent(merged, "", "  ")
		t.Logf("%s/%s (%s, %d turns, %d searches, %d pages read, tokens in/out %d/%d)\nCLAIMED %s\nMERGED %s",
			s.Platform, s.Handle, time.Since(start).Round(time.Second), f.Turns, f.Searches, len(f.Seen),
			u.PromptTokens+u.CacheReadTokens+u.CacheWriteTokens, u.CompletionTokens, claimed, out)
	}
}
