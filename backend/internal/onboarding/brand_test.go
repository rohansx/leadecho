package onboarding

import (
	"strings"
	"testing"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

func TestFromAnalysisBackfillsOldShape(t *testing.T) {
	pa := &ai.ProductAnalysis{
		ProductName:       "Acme",
		Description:       "CRM for agencies",
		TargetAudience:    "agencies, freelancers",
		PainPoints:        []string{"lost deals", "messy spreadsheets"},
		Competitors:       []string{"HubSpot", "hubspot", " "},
		SuggestedKeywords: []string{"Acme CRM", "crm for agencies", "HubSpot"},
		Features:          []string{"pipelines"},
	}
	bp := FromAnalysis(pa, "https://acme.test")

	if bp.Product != "Acme" || bp.Overview != "CRM for agencies" || bp.SourceURL != "https://acme.test" {
		t.Fatalf("basic fields: %+v", bp)
	}
	if len(bp.Competitors) != 1 {
		t.Fatalf("competitors not deduped: %v", bp.Competitors)
	}
	if len(bp.CustomerSegments) != 2 || bp.CustomerSegments[1].Name != "freelancers" {
		t.Fatalf("segments not derived from audience: %+v", bp.CustomerSegments)
	}
	if len(bp.PainPoints) != 2 || bp.PainPoints[0].Problem != "lost deals" {
		t.Fatalf("pain points: %+v", bp.PainPoints)
	}
	kg := bp.Keywords
	if !has(kg.Brand, "Acme") || !has(kg.Brand, "Acme CRM") || !has(kg.Category, "crm for agencies") || !has(kg.Problem, "lost deals") {
		t.Fatalf("keyword groups: %+v", kg)
	}
	if has(kg.Category, "HubSpot") {
		t.Fatal("competitor names must not become category keywords")
	}
}

func TestFromAnalysisKeepsModelGroups(t *testing.T) {
	pa := &ai.ProductAnalysis{
		ProductName:   "Acme",
		KeywordGroups: ai.KeywordGroups{Brand: []string{"acme"}, Problem: []string{"x"}},
	}
	bp := FromAnalysis(pa, "")
	if len(bp.Keywords.Brand) != 1 || bp.Keywords.Problem[0] != "x" {
		t.Fatalf("model groups overwritten: %+v", bp.Keywords)
	}
}

func TestBrandProfileValidate(t *testing.T) {
	long := strings.Repeat("a", 5000)
	cases := []struct {
		name    string
		bp      BrandProfile
		wantErr bool
	}{
		{"ok", BrandProfile{Product: "Acme"}, false},
		{"no product", BrandProfile{Product: "  "}, true},
		{"bad url", BrandProfile{Product: "A", SourceURL: "javascript:alert(1)"}, true},
		{"good url", BrandProfile{Product: "A", SourceURL: "https://a.test"}, false},
		{"long overview truncated", BrandProfile{Product: "A", Overview: long}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.bp.Normalized()
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v", err)
			}
			if err == nil && len(got.Overview) > maxTextLen {
				t.Fatal("overview not capped")
			}
		})
	}
}

func TestBrandProfileRowRoundTrip(t *testing.T) {
	bp := BrandProfile{
		Product:          "Acme",
		CustomerSegments: []ai.CustomerSegment{{Name: "Agencies", WhyTheyBuy: "y"}},
		PainPoints:       []ai.PainPointDetail{{Problem: "p", Explanation: "e"}},
		Keywords:         ai.KeywordGroups{Brand: []string{"acme"}},
		Competitors:      []string{"HubSpot"},
	}
	params := bp.Params("ws1")
	if params.WorkspaceID != "ws1" || params.Features == nil {
		t.Fatalf("params: %+v", params)
	}
	row := rowFromParams(params)
	back := FromRow(row)
	if back.CustomerSegments[0].Name != "Agencies" || back.PainPoints[0].Explanation != "e" || back.Keywords.Brand[0] != "acme" {
		t.Fatalf("round trip: %+v", back)
	}
	if all := back.AllKeywords(); len(all) != 1 {
		t.Fatalf("AllKeywords: %v", all)
	}
}

func rowFromParams(p database.UpsertBrandProfileParams) database.BrandProfile {
	return database.BrandProfile{
		WorkspaceID: p.WorkspaceID, Overview: p.Overview, Product: p.Product, Positioning: p.Positioning,
		CustomerSegments: p.CustomerSegments, Competitors: p.Competitors, PainPoints: p.PainPoints,
		Keywords: p.Keywords, Features: p.Features, TargetAudience: p.TargetAudience, SourceUrl: p.SourceUrl,
	}
}

func has(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
