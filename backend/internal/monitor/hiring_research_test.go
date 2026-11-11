package monitor

import (
	"encoding/json"
	"strings"
	"testing"

	"leadecho/internal/ai"
)

func TestResearchNoteSaysWhatWasFoundAndNothingWhenEmpty(t *testing.T) {
	if got := researchNote(nil); got != "" {
		t.Fatalf("no research: no note, got %q", got)
	}
	b := &ai.ResearchBrief{
		WhatTheyDo: "Sells vertical software to community businesses.",
		WhyNow:     "Moved its HQ to Austin and plans to hire 250 people.",
		Hook:       "Many roles open at once.",
		Facts:      []ai.ResearchFact{{Text: "Plans to hire 250 people.", URL: "https://news.example/a"}},
	}
	got := researchNote(b)
	for _, want := range []string{"What a web search found", "not verified", "Sells vertical software", "Moved its HQ to Austin"} {
		if !strings.Contains(got, want) {
			t.Errorf("note is missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "https://") {
		t.Error("links live in the metadata, not in the lead text")
	}
}

func TestHiringMetadataCarriesTheResearchWithSources(t *testing.T) {
	b := &ai.ResearchBrief{WhatTheyDo: "x", Facts: []ai.ResearchFact{{Text: "f", URL: "https://news.example/a"}}}
	var meta struct {
		Signal   string            `json:"signal"`
		Research *ai.ResearchBrief `json:"research"`
	}
	if err := json.Unmarshal(hiringMetadata(jobPost{Company: "Acme", Title: "Engineer"}, "VP Eng", contactCheck{confirmed: true}, b), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Signal != "hiring" || meta.Research == nil || meta.Research.Facts[0].URL != "https://news.example/a" {
		t.Fatalf("meta = %+v", meta)
	}
	// Without research the key is absent, not null.
	if raw := string(hiringMetadata(jobPost{Company: "Acme"}, "", contactCheck{}, nil)); strings.Contains(raw, "research") {
		t.Fatalf("unexpected research key: %s", raw)
	}
}
