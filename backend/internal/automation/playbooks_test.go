package automation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"leadecho/internal/apikey"
	"leadecho/internal/database"
	"leadecho/internal/tools"
)

var now = time.Date(2026, 11, 4, 9, 30, 0, 0, time.UTC)

func TestDailyTriage(t *testing.T) {
	since := now.Add(-24 * time.Hour)
	ms := []tools.MentionView{
		mv("old", 9, "buy_signal", "new", since.Add(-time.Hour)), // before window
		mv("low", 1.5, "general", "new", now.Add(-time.Hour)),    // noise
		mv("mid", 6, "comparison", "new", now.Add(-2*time.Hour)),
		mv("top", 9, "buy_signal", "new", now.Add(-3*time.Hour)),
		mv("tie", 6, "buy_signal", "new", now.Add(-4*time.Hour)), // same score as mid, stronger intent
		mv("unscored", -1, "", "new", now.Add(-time.Minute)),     // unknown is not noise
	}
	tests := []struct {
		name         string
		cfg          Config
		wantItems    []string
		wantArchived []string
	}{
		{"rank only", Config{TopN: 3, NoiseBelow: 3}, []string{"top", "tie", "mid"}, nil},
		{"archive noise when allowed", Config{TopN: 10, NoiseBelow: 3, MarkNoise: true}, []string{"top", "tie", "mid", "unscored"}, []string{"low"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := &fakeTools{handlers: map[string]func(map[string]any) (any, error){
				"search_mentions": func(a map[string]any) (any, error) {
					if a["status"] != "new" {
						t.Fatalf("args = %v", a)
					}
					return mentionsResult(ms...)
				},
			}}
			fq := &fakeQ{}
			out, err := Runner{Tools: ft, Q: fq}.Run(context.Background(), ws, KindDailyTriage, tt.cfg, since)
			if err != nil {
				t.Fatal(err)
			}
			if got := itemIDs(out.Items); strings.Join(got, ",") != strings.Join(tt.wantItems, ",") {
				t.Fatalf("items = %v, want %v", got, tt.wantItems)
			}
			if strings.Join(names(fq.archived), ",") != strings.Join(tt.wantArchived, ",") {
				t.Fatalf("archived = %v, want %v", fq.archived, tt.wantArchived)
			}
			if !strings.Contains(out.Summary, "5 new mentions") {
				t.Fatalf("summary = %q", out.Summary)
			}
		})
	}
}

func TestLeadSweep(t *testing.T) {
	byIntent := map[string][]tools.MentionView{
		"buy_signal": {
			mv("hot", 8, "buy_signal", "new", now),
			mv("weak", 4, "buy_signal", "new", now),  // below min score
			mv("dup", 7, "buy_signal", "new", now),   // already a lead
			mv("gone", 9, "buy_signal", "spam", now), // excluded status
		},
		"recommendation_ask": {
			mv("ask", 7.5, "recommendation_ask", "reviewed", now),
			mv("hot", 8, "buy_signal", "new", now), // seen twice
		},
	}
	tests := []struct {
		name      string
		cfg       Config
		crmErr    error
		wantLeads []string
		wantPush  int
		wantNote  string
	}{
		{"creates leads", Config{MinScore: 6, MaxLeads: 20}, nil, []string{"hot", "ask"}, 0, ""},
		{"respects cap", Config{MinScore: 6, MaxLeads: 1}, nil, []string{"hot"}, 0, ""},
		{"pushes to crm", Config{MinScore: 6, MaxLeads: 20, PushToCRM: true}, nil, []string{"hot", "ask"}, 2, ""},
		{"crm not configured is a note", Config{MinScore: 6, MaxLeads: 20, PushToCRM: true}, &tools.Error{Msg: "CRM sync is not configured on this server"}, []string{"hot", "ask"}, 1, "CRM sync is not configured"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := &fakeTools{handlers: map[string]func(map[string]any) (any, error){
				"search_mentions": func(a map[string]any) (any, error) { return mentionsResult(byIntent[a["intent"].(string)]...) },
				"push_to_crm": func(a map[string]any) (any, error) {
					if tt.crmErr != nil {
						return nil, tt.crmErr
					}
					return map[string]any{"queued": true}, nil
				},
			}}
			fq := &fakeQ{leadsFor: map[string]bool{"dup": true}}
			out, err := Runner{Tools: ft, Q: fq}.Run(context.Background(), ws, KindLeadSweep, tt.cfg, now.Add(-24*time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, c := range fq.created {
				if c.WorkspaceID != ws || c.Stage != database.LeadStageProspect || !c.MentionID.Valid {
					t.Fatalf("bad lead params %+v", c)
				}
				got = append(got, c.Username.String[len("author-"):])
			}
			if strings.Join(got, ",") != strings.Join(tt.wantLeads, ",") {
				t.Fatalf("leads = %v, want %v", got, tt.wantLeads)
			}
			pushes := ft.called("push_to_crm")
			if len(pushes) != tt.wantPush {
				t.Fatalf("pushes = %d, want %d", len(pushes), tt.wantPush)
			}
			for _, p := range pushes {
				if !contains(p.Scopes, apikey.ScopeWrite) {
					t.Fatalf("push_to_crm needs write scope, got %v", p.Scopes)
				}
			}
			if tt.wantNote != "" && !strings.Contains(strings.Join(out.Notes, ";"), tt.wantNote) {
				t.Fatalf("notes = %v", out.Notes)
			}
		})
	}
}

func TestAttributionReview(t *testing.T) {
	ft := &fakeTools{handlers: map[string]func(map[string]any) (any, error){
		"attribution_report": func(a map[string]any) (any, error) {
			if a["days"] != float64(7) {
				t.Fatalf("days = %v", a["days"])
			}
			return map[string]any{"totals": map[string]any{"clicks": 12, "revenue": "$49.00"}}, nil
		},
	}}
	fq := &fakeQ{kwRows: []database.AutomationKeywordValueRow{
		{ID: "k1", Term: "crm alternative", Mentions: 10, RepliesPosted: 3, Clicks: 12, RevenueCents: 4900},
		{ID: "k2", Term: "ai", Mentions: 40},   // noisy and worthless: suggest mute
		{ID: "k3", Term: "quiet", Mentions: 2}, // below min_mentions: leave alone
		{ID: "k4", Term: "signup only", Mentions: 6, Signups: 1},
	}}
	out, err := Runner{Tools: ft, Q: fq}.Run(context.Background(), ws, KindAttributionReview, Config{Days: 7, MinMentions: 5}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 2 || out.Items[0].Title != "crm alternative" || out.Items[1].Title != "signup only" {
		t.Fatalf("items = %+v", out.Items)
	}
	if len(out.Suggestions) != 1 || !strings.Contains(out.Suggestions[0], `"ai"`) {
		t.Fatalf("suggestions = %v", out.Suggestions)
	}
	if !strings.Contains(out.Summary, "$49.00") {
		t.Fatalf("summary = %q", out.Summary)
	}
}

func TestCustomSteps(t *testing.T) {
	ft := &fakeTools{handlers: map[string]func(map[string]any) (any, error){
		"weekly_summary": func(map[string]any) (any, error) { return map[string]any{"headline": "ok"}, nil },
		"get_lead":       func(map[string]any) (any, error) { return nil, &tools.Error{Msg: "lead not found"} },
	}}
	cfg := Config{Steps: []Step{{Tool: "weekly_summary", Args: []byte(`{}`)}, {Tool: "get_lead", Args: []byte(`{"lead_id":"x"}`)}}}
	out, err := Runner{Tools: ft, Q: &fakeQ{}}.Run(context.Background(), ws, KindCustom, cfg, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range ft.calls {
		if len(c.Scopes) != 1 || c.Scopes[0] != apikey.ScopeRead {
			t.Fatalf("custom steps must be read-only, got %v", c.Scopes)
		}
	}
	if !strings.Contains(out.Summary, "1 failed") || !strings.Contains(strings.Join(out.Notes, ";"), "lead not found") {
		t.Fatalf("out = %+v", out)
	}

	// internal (non-user) errors fail the run
	ft.handlers["weekly_summary"] = func(map[string]any) (any, error) { return nil, errors.New("db down") }
	if _, err := (Runner{Tools: ft, Q: &fakeQ{}}).Run(context.Background(), ws, KindCustom, cfg, now); err == nil {
		t.Fatal("want error")
	}
}

func itemIDs(items []Item) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, idNames[it.MentionID])
	}
	return out
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func names(ids []string) []string {
	out := []string{}
	for _, id := range ids {
		out = append(out, idNames[id])
	}
	return out
}
