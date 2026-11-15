//go:build live

// Writes real drafts for leads exported from a workspace, with the contact
// names replaced, and checks them against what reviewers found wrong in
// production (a place or figure that is not the posting's, free work, an
// English email for a Spanish posting, a reply that argues for the approach
// the product replaces). Spends a few cents. Run:
//
//	ANTHROPIC_API_KEY=... DRAFTS_LEADS_JSON=/path/leads.json go test -tags live -run TestDraftsLive -v ./internal/ai/
//
// The export is not in the repo: it names real companies. EVAL_MODEL
// overrides the model (default: the one drafts run on).
package ai

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

type liveLead struct {
	ID       string  `json:"id"`
	Signal   *string `json:"signal"`
	Platform string  `json:"platform"`
	Intent   string  `json:"intent"`
	Title    string  `json:"title"`
	Content  string  `json:"content"`
	Research *struct {
		WhatTheyDo string         `json:"what_they_do"`
		WhyNow     string         `json:"why_now"`
		Hook       string         `json:"hook"`
		Facts      []ResearchFact `json:"facts"`
	} `json:"research"`
}

var (
	liveJobRE     = regexp.MustCompile(`^(.+?) posted a job for "(.+?)"(?: \((.+)\))? this week\.`)
	liveContactRE = regexp.MustCompile(`Who to contact: [^,]+, (.+?) at `)
)

const (
	liveProduct     = "Utkrusht"
	liveOverview    = "A technical skill assessment platform for hiring software engineers. Instead of coding tests or take-homes, candidates solve real problems in a live, running system; Utkrusht records how they reason, debug and use AI, and hands the hiring team a ranked shortlist of the best 5-10 with recordings and scored reports."
	livePositioning = "Real problems in a live, running system instead of coding tests or take-home assignments."
)

func TestDraftsLive(t *testing.T) {
	key, path := os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("DRAFTS_LEADS_JSON")
	if key == "" || path == "" {
		t.Skip("ANTHROPIC_API_KEY and DRAFTS_LEADS_JSON not set")
	}
	model := os.Getenv("EVAL_MODEL")
	if model == "" {
		model = "claude-opus-5-5"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	leads := map[string]liveLead{}
	for k, v := range doc {
		if !strings.Contains(k, "INBOX LEADS") {
			continue
		}
		var list []liveLead
		if err := json.Unmarshal(v, &list); err != nil {
			t.Fatal(err)
		}
		for _, l := range list {
			leads[l.ID] = l
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	p := Provider{Name: "anthropic", APIKey: key, Model: model}
	today := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)

	// Each case: the lead, an invented contact, and what its production draft got wrong.
	for _, c := range []struct {
		id, contact   string
		proof, sign   string
		spanish       bool
		never         []string // must not appear (case-insensitive)
		wantSubstring string
	}{
		{id: "eced492c", contact: "Dana Whitfield", never: []string{"57"}},                                             // quoted "57 open roles" from a headcount tracker
		{id: "72a0c2cd", contact: "Priya Raman", never: []string{"palo alto", "developer experience", "productivity"}}, // pitched another role in another city
		{id: "3e6e67e2", contact: "Marcus Bell", never: []string{"mesa", "richardson", "durham", "4,913"}},             // cities from an unrelated page
		{id: "79ee2884", contact: "Lena Fischer", never: []string{"could i run", "can i run", "free"}},                 // offered free work
		{id: "1ad39b1c", contact: "Carlos Mendez", spanish: true},
		{id: "d63d90f3", contact: "Lucia Paredes", spanish: true},
		{id: "9cb1349f", contact: "Diego Salas", spanish: true, never: []string{"2 mill"}, proof: "Northwind cut engineer interview hours by 30% in its first month.", sign: "Asha Verma\nFounder, Utkrusht", wantSubstring: "30%"},
		{id: "190b7e38", contact: "Tom Okafor", sign: "Asha Verma\nFounder, Utkrusht", wantSubstring: "Asha Verma\nFounder, Utkrusht"},
	} {
		l, ok := leads[c.id]
		if !ok {
			t.Errorf("%s: not in the export", c.id)
			continue
		}
		job := liveJobRE.FindStringSubmatch(l.Content)
		title := liveContactRE.FindStringSubmatch(l.Content)
		if job == nil || title == nil {
			t.Errorf("%s: cannot read the posting from the lead text", c.id)
			continue
		}
		o := Outreach{
			Product: liveProduct, Overview: liveOverview, Positioning: livePositioning,
			ContactName: c.contact, ContactTitle: title[1], Company: job[1],
			JobTitle: job[2], JobLocation: job[3], JobPosted: today.AddDate(0, 0, -2), Today: today,
			ProofPoint: c.proof, SignOff: c.sign,
		}
		if l.Research != nil {
			o.Research = &ResearchBrief{WhatTheyDo: l.Research.WhatTheyDo, WhyNow: l.Research.WhyNow, Hook: l.Research.Hook, Facts: l.Research.Facts}
		}
		out, err := DraftOutreach(ctx, p, o)
		if err != nil {
			t.Errorf("%s: %v", c.id, err)
			continue
		}
		t.Logf("----- %s: %s / %s (%s)\n%s", c.id, o.Company, o.JobTitle, o.JobLocation, out)
		low := strings.ToLower(out)
		for _, n := range append(c.never, "sample report", "free trial", "pilot", "discount", "http") {
			if strings.Contains(low, n) {
				t.Errorf("%s: the draft says %q", c.id, n)
			}
		}
		if c.wantSubstring != "" && !strings.Contains(out, c.wantSubstring) {
			t.Errorf("%s: the draft lacks %q", c.id, c.wantSubstring)
		}
		if c.proof == "" && strings.Contains(low, "northwind") {
			t.Errorf("%s: a customer was named without a proof point", c.id)
		}
		if isSpanish := strings.Contains(low, "hola") || strings.Contains(low, "estimad") || strings.Contains(low, " que "); isSpanish != c.spanish {
			t.Errorf("%s: Spanish=%v, want %v", c.id, isSpanish, c.spanish)
		}
		if c.sign == "" && !strings.HasSuffix(strings.TrimSpace(out), "?") {
			t.Errorf("%s: no sign-off was given, so the message should end on its question", c.id)
		}
	}

	// The public reply that coached a hiring manager on take-homes and ended
	// in a bare tracking link: every style, since the style is picked at random.
	post, ok := leads["9b6b16d2"]
	if !ok {
		t.Fatal("9b6b16d2: not in the export")
	}
	for _, style := range []string{"casual_helpful", "value_first", "storytelling"} {
		res, err := DraftReplyEnhanced(ctx, p, DraftReplyOptions{
			Title: post.Title, Content: post.Content, Platform: post.Platform, Intent: post.Intent,
			AwarenessLevel: "problem_aware", TemplateStyle: style, ProductName: liveProduct,
			ProductDescription: liveOverview, Positioning: livePositioning,
			Voice: "- Goal: move the reader toward trying the product, without being pushy.\n- Product mentions: Mention the product at most once, briefly, as one option, only after giving real help.",
		})
		if err != nil {
			t.Errorf("reply (%s): %v", style, err)
			continue
		}
		t.Logf("----- reply to 9b6b16d2 (%s)\n%s", style, res.Reply)
		low := strings.ToLower(res.Reply)
		if strings.Contains(low, "http") || strings.Contains(low, "/r/") {
			t.Errorf("reply (%s): carries a link", style)
		}
		if first, _, _ := strings.Cut(res.Reply, "."); strings.Contains(strings.ToLower(first), "utkrusht") {
			t.Errorf("reply (%s): names the product in the first sentence", style)
		}
	}
}
