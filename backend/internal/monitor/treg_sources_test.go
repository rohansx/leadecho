package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/treg"
)

const tregTestWS = "11111111-2222-3333-4444-555555555555"

func tregMonitor(t *testing.T, h http.HandlerFunc) *Monitor {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	m := &Monitor{logger: zerolog.Nop()}
	m.SetTreg(treg.New(treg.Config{Token: "tok", BaseURL: srv.URL}, nil, zerolog.Nop()))
	return m
}

func TestTregSearchRequestAndParse(t *testing.T) {
	var path string
	var body map[string]string
	m := tregMonitor(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"output":{"posts":[{"id":"1","text":"need a crm that is not salesforce","author":{"userName":"a"}}]}}`))
	})
	posts, ok := m.tregSearch(context.Background(), tregTestWS, tregXTool, map[string]string{"q": "crm"})
	if !ok || len(posts) != 1 || posts[0].ID != "1" {
		t.Fatalf("ok=%v posts=%+v", ok, posts)
	}
	if path != "/call/"+tregXTool || body["q"] != "crm" {
		t.Fatalf("path=%s body=%v", path, body)
	}
}

func TestCrawlTregFallbackSignals(t *testing.T) {
	kw := database.ListActiveKeywordsRow{ID: "8a3c3b7e-8c1d-4c9e-9f1a-1b2c3d4e5f60", Term: "zebra-unmatched-term", MatchType: "phrase"}
	cases := []struct {
		name   string
		status int
		body   string
		wantOK bool
	}{
		{"balance empty falls back", 402, `{"error":"insufficient_balance"}`, false},
		{"provider error falls back", 400, `{}`, false},
		{"answered with no matches does not fall back", 200, `{"output":{"posts":[{"id":"1","text":"nothing relevant in this post at all"}]}}`, true},
		{"miss does not fall back", 200, `{"output":{"posts":[]}}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := tregMonitor(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			for _, crawl := range []func(context.Context, string, database.ListActiveKeywordsRow) ([]mentionAlert, bool){m.crawlTwitterTreg, m.crawlLinkedInTreg} {
				alerts, ok := crawl(context.Background(), tregTestWS, kw)
				if ok != tc.wantOK || len(alerts) != 0 {
					t.Fatalf("ok=%v alerts=%d, want ok=%v", ok, len(alerts), tc.wantOK)
				}
			}
		})
	}
}

func TestCrawlTregDisabled(t *testing.T) {
	m := &Monitor{logger: zerolog.Nop()}
	if _, ok := m.crawlTwitterTreg(context.Background(), tregTestWS, database.ListActiveKeywordsRow{Term: "x"}); ok {
		t.Fatal("disabled treg must report ok=false")
	}
}
