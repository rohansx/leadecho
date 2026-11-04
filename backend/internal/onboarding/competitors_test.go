package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompetitorNameFromResult(t *testing.T) {
	cases := []struct{ title, url, want string }{
		{"Pipedrive - Sales CRM", "https://www.pipedrive.com/", "Pipedrive"},
		{"Close | The Inside Sales CRM", "https://close.com", "Close"},
		{"", "https://www.folk.app/", "Folk"},
		{"Top 10 HubSpot alternatives in 2026", "https://www.g2.com/x", ""}, // aggregator
		{"Acme: home", "https://acme.test", ""},                             // own product
	}
	for _, c := range cases {
		if got := competitorName(c.title, c.url, "Acme", "acme.test"); got != c.want {
			t.Errorf("%q %q: got %q want %q", c.title, c.url, got, c.want)
		}
	}
}

func TestExaCompetitorFinder(t *testing.T) {
	var gotBody map[string]any
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		fmt.Fprint(w, `{"results":[
			{"title":"Pipedrive - CRM","url":"https://pipedrive.com"},
			{"title":"Pipedrive pricing","url":"https://pipedrive.com/pricing"},
			{"title":"Best CRMs","url":"https://capterra.com/crm"},
			{"title":"Attio: CRM","url":"https://attio.com"}
		]}`)
	}))
	defer srv.Close()

	f := &ExaCompetitorFinder{APIKey: "k", Endpoint: srv.URL}
	got, err := f.FindCompetitors(context.Background(), "Acme", "https://acme.test")
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "k" || gotBody["query"] != "Acme alternatives" || gotBody["category"] != "company" {
		t.Fatalf("request: key=%q body=%v", gotKey, gotBody)
	}
	if len(got) != 2 || got[0] != "Pipedrive" || got[1] != "Attio" {
		t.Fatalf("competitors: %v", got)
	}
}

func TestFindCompetitorsNilSafe(t *testing.T) {
	var f *ExaCompetitorFinder
	if got, err := f.FindCompetitors(context.Background(), "Acme", ""); err != nil || got != nil {
		t.Fatalf("nil finder: %v %v", got, err)
	}
	if got, err := (&ExaCompetitorFinder{}).FindCompetitors(context.Background(), "Acme", ""); err != nil || got != nil {
		t.Fatalf("keyless finder: %v %v", got, err)
	}
	if NewCompetitorFinder("") != nil {
		t.Fatal("no key must mean no finder")
	}
}
