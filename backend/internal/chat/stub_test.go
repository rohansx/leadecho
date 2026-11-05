package chat

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/tools"
)

func TestStubModelKeylessAnswers(t *testing.T) {
	user := "jane"
	tests := []struct {
		name      string
		question  string
		wantCalls []string
		wantIn    []string
	}{
		{
			name: "best leads uses search_mentions and list_leads", question: "Show me today's best leads",
			wantCalls: []string{"search_mentions", "list_leads"},
			wantIn:    []string{"Looking for a Mention alternative", "jane", "qualified", "No AI provider"},
		},
		{
			name: "money question uses attribution_report", question: "Why did this keyword make money?",
			wantCalls: []string{"attribution_report"},
			wantIn:    []string{"$12.50", "reddit"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ft := &fakeTools{results: map[string]any{
				"search_mentions": sampleMentions(),
				"list_leads":      map[string]any{"leads": []tools.LeadView{{ID: "l1", Stage: "qualified", Username: &user}}},
				"attribution_report": map[string]any{
					"days":        30,
					"by_platform": []map[string]any{{"platform": "reddit", "replies_posted": 3, "clicks": 9, "signups": 1, "revenue_cents": 1250}},
					"totals":      map[string]any{"replies_posted": 3, "clicks": 9, "signups": 1, "revenue": "$12.50"},
				},
			}}
			a := &Agent{Model: StubModel{}, Tools: ft, MaxSteps: 6, StepTimeout: time.Second}
			turn, err := a.Run(context.Background(), "ws", []ai.ChatMessage{{Role: "user", Content: tc.question}}, func(string, any) {})
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, c := range ft.calls {
				names = append(names, c.name)
			}
			if !slices.Equal(names, tc.wantCalls) {
				t.Fatalf("calls %v, want %v", names, tc.wantCalls)
			}
			for _, s := range tc.wantIn {
				if !strings.Contains(turn.Answer, s) {
					t.Errorf("answer %q lacks %q", turn.Answer, s)
				}
			}
		})
	}
}
