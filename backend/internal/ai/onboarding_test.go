package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeLLM serves an OpenAI-compatible /chat/completions that answers content.
func fakeLLM(t *testing.T, content string) Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(content)
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}]}`, b)
	}))
	t.Cleanup(srv.Close)
	return Provider{Name: "openai", APIKey: "k", BaseURL: srv.URL, Model: "m"}
}

func TestAnalyzeProductPageExtendedFields(t *testing.T) {
	p := fakeLLM(t, "```json\n"+`{
		"product_name":"Acme","description":"CRM for agencies","features":["pipelines"],
		"target_audience":["agencies","freelancers"],
		"pain_points":["messy spreadsheets"],
		"pain_point_details":[{"problem":"messy spreadsheets","explanation":"deals get lost"}],
		"customer_segments":[{"name":"Agencies","why_they_buy":"client pipelines"}],
		"positioning":"simplest CRM",
		"keyword_groups":{"brand":["Acme"],"product":["acme crm"],"category":["crm for agencies"],"problem":["lost deals"]},
		"competitors":["HubSpot"],"suggested_keywords":["acme"]
	}`+"\n```")
	pa, err := AnalyzeProductPage(context.Background(), p, "page")
	if err != nil {
		t.Fatal(err)
	}
	if string(pa.TargetAudience) != "agencies, freelancers" {
		t.Fatalf("target audience: %q", pa.TargetAudience)
	}
	if len(pa.CustomerSegments) != 1 || pa.CustomerSegments[0].WhyTheyBuy != "client pipelines" {
		t.Fatalf("segments: %+v", pa.CustomerSegments)
	}
	if pa.KeywordGroups.Category[0] != "crm for agencies" || pa.PainPointDetails[0].Explanation != "deals get lost" || pa.Positioning != "simplest CRM" {
		t.Fatalf("extended fields missing: %+v", pa)
	}
}

func TestAnalyzeProductPageOldShapeStillParses(t *testing.T) {
	p := fakeLLM(t, `{"product_name":"Old","pain_points":["a"],"suggested_keywords":["b"]}`)
	pa, err := AnalyzeProductPage(context.Background(), p, "page")
	if err != nil || pa.ProductName != "Old" || len(pa.CustomerSegments) != 0 {
		t.Fatalf("backward compat broken: %+v %v", pa, err)
	}
}

func TestGenerateListener(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr bool
		wantKW  int
	}{
		{"valid", `{"name":"Buyers","keywords":["best crm","crm recommendation"],"looks_for":"people shopping","skips":"vendors"}`, false, 2},
		{"fenced", "```json\n{\"name\":\"B\",\"keywords\":[\"x\"],\"looks_for\":\"l\",\"skips\":\"s\"}\n```", false, 1},
		{"no keywords", `{"name":"B","keywords":[],"looks_for":"l"}`, true, 0},
		{"garbage", `not json`, true, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := GenerateListener(context.Background(), fakeLLM(t, c.content), ListenerPrompt{Goal: "find_buyers", Product: "Acme"})
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, c.wantErr)
			}
			if err == nil && len(got.Keywords) != c.wantKW {
				t.Fatalf("keywords %v", got.Keywords)
			}
		})
	}
}

func TestListenerPromptText(t *testing.T) {
	s := ListenerPrompt{Goal: "competitor_switchers", Product: "Acme", Competitors: []string{"HubSpot"}, PainPoints: []string{"lost deals"}}.userText()
	for _, want := range []string{"competitor_switchers", "Acme", "HubSpot", "lost deals"} {
		if !strings.Contains(s, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}
