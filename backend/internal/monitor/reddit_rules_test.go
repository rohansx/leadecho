package monitor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSubredditRules(t *testing.T) {
	rulesJSON := `{"rules":[{"short_name":"No self-promotion","description":"No ads."}],"site_rules":["Spam"]}`
	aboutJSON := `{"data":{"public_description":"For founders","description":"Weekly promo thread on Mondays","submit_text":""}}`
	routes := func(oauth bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/access_token" {
				_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
				return
			}
			suffix := ""
			if !oauth {
				suffix = ".json"
			}
			switch r.URL.Path {
			case "/r/SaaS/about/rules" + suffix:
				_, _ = w.Write([]byte(rulesJSON))
			case "/r/SaaS/about" + suffix:
				_, _ = w.Write([]byte(aboutJSON))
			default:
				w.WriteHeader(http.StatusForbidden)
			}
		}
	}
	for _, oauth := range []bool{false, true} {
		srv := httptest.NewServer(routes(oauth))
		s := &PublicSources{Client: srv.Client(), RedditBase: srv.URL}
		if oauth {
			s.RedditOAuth = NewRedditOAuth("cid", "secret", "web:leadecho:1.0", srv.URL, srv.URL)
		}
		raw, err := s.SubredditRules(context.Background(), "SaaS")
		srv.Close()
		if err != nil || len(raw.Rules) != 1 || raw.Rules[0].ShortName != "No self-promotion" {
			t.Fatalf("oauth=%v: raw=%+v err=%v", oauth, raw, err)
		}
		if raw.Sidebar != "For founders\nWeekly promo thread on Mondays\n" {
			t.Fatalf("oauth=%v: sidebar=%q", oauth, raw.Sidebar)
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer srv.Close()
	_, err := (&PublicSources{Client: srv.Client(), RedditBase: srv.URL}).SubredditRules(context.Background(), "SaaS")
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusForbidden {
		t.Fatalf("blocked Reddit should surface the 403, got %v", err)
	}
}
