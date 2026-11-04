package monitor

import (
	"context"
	"errors"
	"testing"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

type fakeExamples struct {
	ex  []ai.ScoringExample
	err error
	got struct{ ws, profile string }
}

func (f *fakeExamples) ScoringExamples(_ context.Context, ws, profile string, limit int) ([]ai.ScoringExample, error) {
	f.got.ws, f.got.profile = ws, profile
	if len(f.ex) > limit {
		return f.ex[:limit], f.err
	}
	return f.ex, f.err
}

func TestCollectExamples(t *testing.T) {
	mk := func(n int, label string) []ai.ScoringExample {
		out := make([]ai.ScoringExample, n)
		for i := range out {
			out[i] = ai.ScoringExample{Label: label, Content: "c"}
		}
		return out
	}
	tests := []struct {
		name    string
		sources []ExampleSource
		wantN   int
		wantUp  int
	}{
		{"none", nil, 0, 0},
		{"feedback only", []ExampleSource{&fakeExamples{ex: mk(3, "up")}}, 3, 3},
		{"capped across sources", []ExampleSource{&fakeExamples{ex: mk(4, "up")}, &fakeExamples{ex: mk(5, "down")}}, ai.MaxScoringExamples, 4},
		{"failing source is skipped", []ExampleSource{&fakeExamples{err: errors.New("boom")}, &fakeExamples{ex: mk(2, "down")}}, 2, 0},
		{"invalid labels dropped", []ExampleSource{&fakeExamples{ex: []ai.ScoringExample{{Label: "meh"}, {Label: "up"}}}}, 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectExamples(context.Background(), tt.sources, "ws", "p1", ai.MaxScoringExamples, func(error) {})
			up := 0
			for _, e := range got {
				if e.Label == "up" {
					up++
				}
			}
			if len(got) != tt.wantN || up != tt.wantUp {
				t.Fatalf("got %d examples (%d up), want %d (%d up)", len(got), up, tt.wantN, tt.wantUp)
			}
		})
	}
}

func TestCollectExamplesPassesProfile(t *testing.T) {
	f := &fakeExamples{}
	collectExamples(context.Background(), []ExampleSource{f}, "ws-9", "prof-2", 6, func(error) {})
	if f.got.ws != "ws-9" || f.got.profile != "prof-2" {
		t.Fatalf("source called with %+v", f.got)
	}
}

func TestBuildScoringContext(t *testing.T) {
	ex := []ai.ScoringExample{{Label: "up"}}
	sc := buildScoringContext(&database.GetMentionScoringProfileRow{
		Name: "Uptime", Description: "monitoring for SaaS", LooksFor: " teams leaving Pingdom ", Skips: "students",
	}, ex)
	if sc.Product != "- Uptime: monitoring for SaaS" || sc.LooksFor != "teams leaving Pingdom" || sc.Skips != "students" || len(sc.Examples) != 1 {
		t.Fatalf("unexpected context: %+v", sc)
	}
	if empty := buildScoringContext(nil, nil); empty.Product != "" || empty.LooksFor != "" {
		t.Fatalf("nil profile should give generic context, got %+v", empty)
	}
}
