package onboarding

import (
	"context"
	"errors"
	"strings"
	"testing"

	"leadecho/internal/database"
)

func TestHeuristicAnalysis(t *testing.T) {
	cases := []struct {
		name     string
		pages    []Page
		wantName string
	}{
		{"title", []Page{{URL: "https://acme.test", Title: "Acme | CRM for agencies", Text: "Acme keeps deals moving. More text."}}, "Acme"},
		{"host", []Page{{URL: "https://www.zed.dev/x", Text: "Fast editor."}}, "Zed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pa := HeuristicAnalysis(c.pages)
			if pa.ProductName != c.wantName || len(pa.KeywordGroups.Brand) != 1 || pa.Description == "" {
				t.Fatalf("%+v", pa)
			}
		})
	}
	if pa := HeuristicAnalysis(nil); pa.ProductName != "" {
		t.Fatal("no pages must give empty analysis")
	}
}

func TestAnalysisText(t *testing.T) {
	pages := []Page{{URL: "https://a.test", Text: "home"}, {URL: "https://a.test/pricing", Title: "Pricing", Text: strings.Repeat("p", 10000)}}
	got := AnalysisText(pages)
	if !strings.HasPrefix(got, "home") || !strings.Contains(got, "Pricing") || len(got) > maxAnalysisText {
		t.Fatalf("len=%d prefix=%q", len(got), got[:20])
	}
}

type fakeFeedbackStore struct {
	rows []database.OnboardingSampleFeedback
	err  error
	got  database.ListSampleFeedbackForProfileParams
}

func (f *fakeFeedbackStore) ListSampleFeedbackForProfile(_ context.Context, p database.ListSampleFeedbackForProfileParams) ([]database.OnboardingSampleFeedback, error) {
	f.got = p
	return f.rows, f.err
}

func TestListCalibrationExamples(t *testing.T) {
	store := &fakeFeedbackStore{rows: []database.OnboardingSampleFeedback{
		{Url: "u1", Text: "t1", Label: "up", Platform: "reddit", Goal: "find_buyers"},
		{Url: "u2", Text: "t2", Label: "down"},
	}}
	got, err := ListCalibrationExamples(context.Background(), store, "ws", "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if store.got.WorkspaceID != "ws" || !store.got.ProfileID.Valid {
		t.Fatalf("query must be workspace + profile scoped: %+v", store.got)
	}
	if len(got) != 2 || !got[0].Relevant || got[1].Relevant || got[0].Text != "t1" {
		t.Fatalf("%+v", got)
	}
	if _, err := ListCalibrationExamples(context.Background(), store, "ws", "not-a-uuid"); err == nil {
		t.Fatal("bad profile id must error")
	}
	store.err = errors.New("db down")
	if _, err := ListCalibrationExamples(context.Background(), store, "ws", "00000000-0000-0000-0000-000000000001"); err == nil {
		t.Fatal("db error must propagate")
	}
}

func TestCalibrationSource(t *testing.T) {
	store := &fakeFeedbackStore{rows: []database.OnboardingSampleFeedback{
		{Platform: "reddit", Text: "need a tool for this", Label: "up"},
		{Platform: "hackernews", Text: "Show HN: my thing", Label: "down"},
		{Platform: "devto", Text: "third", Label: "up"},
	}}
	src := CalibrationSource{Store: store}
	got, err := src.ScoringExamples(context.Background(), "ws", "00000000-0000-0000-0000-000000000001", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Label != "up" || got[1].Label != "down" || got[0].Content != "need a tool for this" {
		t.Fatalf("got %+v", got)
	}
	if got, err := src.ScoringExamples(context.Background(), "ws", "", 6); err != nil || got != nil {
		t.Fatalf("workspace-level: %v %v", got, err)
	}
}
