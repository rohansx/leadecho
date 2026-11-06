package workflow

import (
	"strings"
	"testing"
)

func TestTriggerMatches(t *testing.T) {
	lead := Lead{
		Platform: "reddit", Title: "Looking for an alternative to Acme", Content: "Acme keeps breaking, what do you use?",
		Intent: "recommendation_ask", IntentTags: []string{"seeking_alternative", "competitor_complaint"}, Score: 7.5,
	}
	tests := []struct {
		name string
		t    Trigger
		want bool
	}{
		{"empty matches all", Trigger{}, true},
		{"score at threshold", Trigger{MinScore: 7.5}, true},
		{"score below threshold", Trigger{MinScore: 8}, false},
		{"platform any-of (case-insensitive)", Trigger{Platforms: []string{"hackernews", "Reddit"}}, true},
		{"platform miss", Trigger{Platforms: []string{"twitter"}}, false},
		{"intent tag any-of", Trigger{IntentTags: []string{"buying_intent", "competitor_complaint"}}, true},
		{"intent tag miss", Trigger{IntentTags: []string{"hiring"}}, false},
		{"legacy intent type", Trigger{IntentTypes: []string{"recommendation_ask"}}, true},
		{"tags win over legacy types", Trigger{IntentTags: []string{"hiring"}, IntentTypes: []string{"recommendation_ask"}}, false},
		{"keyword in title", Trigger{Keywords: []string{"ALTERNATIVE"}}, true},
		{"keyword miss", Trigger{Keywords: []string{"pricing"}}, false},
		{"exclude keyword", Trigger{ExcludeKeywords: []string{"breaking"}}, false},
		{"blank exclude ignored", Trigger{ExcludeKeywords: []string{"  "}}, true},
		{"all filters", Trigger{MinScore: 6, Platforms: []string{"reddit"}, IntentTags: []string{"seeking_alternative"}, Keywords: []string{"acme"}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.t.Matches(lead); got != tc.want {
				t.Fatalf("Matches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDefinitionNormalize(t *testing.T) {
	ok := []ActionConfig{{Type: ActionNotifySlack}}
	tests := []struct {
		name    string
		in      Definition
		wantErr string
		check   func(t *testing.T, d Definition)
	}{
		{
			name: "defaults and cleanup",
			in:   Definition{Name: "  Hot buyers ", Actions: ok, Trigger: Trigger{Keywords: []string{" crm ", "CRM", ""}, IntentTypes: []string{"x"}}},
			check: func(t *testing.T, d Definition) {
				if d.Name != "Hot buyers" || d.MaxPerHour != DefaultPerHour || len(d.Trigger.Keywords) != 1 || d.Trigger.IntentTypes != nil {
					t.Fatalf("got %+v", d)
				}
			},
		},
		{name: "cap clamped", in: Definition{Name: "a", Actions: ok, MaxPerHour: 10_000}, check: func(t *testing.T, d Definition) {
			if d.MaxPerHour != maxPerHour {
				t.Fatalf("max_per_hour = %d", d.MaxPerHour)
			}
		}},
		{name: "empty name", in: Definition{Name: " ", Actions: ok}, wantErr: "name"},
		{name: "no actions", in: Definition{Name: "a"}, wantErr: "actions"},
		{name: "unknown action", in: Definition{Name: "a", Actions: []ActionConfig{{Type: "post_reply"}}}, wantErr: "unknown action"},
		{name: "duplicate action", in: Definition{Name: "a", Actions: []ActionConfig{{Type: ActionAIDraft}, {Type: ActionAIDraft}}}, wantErr: "twice"},
		{name: "bad score", in: Definition{Name: "a", Actions: ok, Trigger: Trigger{MinScore: 11}}, wantErr: "min_score"},
		{name: "webhook needs https", in: Definition{Name: "a", Actions: []ActionConfig{{Type: ActionWebhook, Config: map[string]any{"url": "http://x.example"}}}}, wantErr: "https"},
		{name: "archive + draft conflict", in: Definition{Name: "a", Actions: []ActionConfig{{Type: ActionArchive}, {Type: ActionAIDraft}}}, wantErr: "archive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.in.Normalize()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, got)
		})
	}
}

func TestSlackEscape(t *testing.T) {
	if got := slackEscape("<!channel> & <https://evil|x>"); strings.ContainsAny(got, "<>") {
		t.Fatalf("unescaped: %q", got)
	}
}
