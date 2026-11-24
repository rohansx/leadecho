package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"

	"leadecho/internal/billing"
	"leadecho/internal/subrules"
	"leadecho/internal/treg"
)

type refusingMeter struct{ err error }

func (m refusingMeter) Spend(context.Context, string, int, string) error { return m.err }
func (refusingMeter) Refund(context.Context, string, int, string)        {}

func TestTregSubredditRulesRefusal(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/call/"+tregPageTool && calls.Load() == 1 {
			w.WriteHeader(http.StatusBadGateway) // the provider failed: a real failure
			_, _ = w.Write([]byte(`{"error":"route_failed"}`))
			return
		}
		_, _ = w.Write([]byte(`{"markdown":"1. No self-promotion of any kind here."}`))
	}))
	defer srv.Close()

	// A metered client refusing the workspace: wrapped as ErrRefused, never fetched.
	for _, refusal := range []error{billing.ErrNoPaidData, billing.ErrNoCredits, billing.ErrPaidDataPaused} {
		tc := treg.New(treg.Config{Token: "tok", BaseURL: srv.URL}, nil, zerolog.Nop())
		tc.SetMeter(refusingMeter{refusal})
		_, err := tregSubredditRules(context.Background(), tc, tregTestWS, "SaaS")
		if !errors.Is(err, subrules.ErrRefused) || !errors.Is(err, refusal) {
			t.Fatalf("%v: got %v, want ErrRefused", refusal, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("a refused call reached treg %d times", calls.Load())
	}

	// The overhead meter does not ask for paid data or credits (credits off here).
	tc := treg.New(treg.Config{Token: "tok", BaseURL: srv.URL}, nil, zerolog.Nop())
	tc.SetMeter(billing.OverheadMeter{Credits: billing.New(nil, true)})
	_, err := tregSubredditRules(context.Background(), tc, tregTestWS, "SaaS")
	if err == nil || errors.Is(err, subrules.ErrRefused) {
		t.Fatalf("an upstream failure must stay cacheable, got %v", err)
	}
	if raw, err := tregSubredditRules(context.Background(), tc, tregTestWS, "SaaS"); err != nil || len(raw.Rules) != 1 {
		t.Fatalf("overhead call: raw=%+v err=%v", raw, err)
	}
}

// A scraped old.reddit rules page, as crawl4ai.web.scrape answers it.
const oldRedditRulesMD = `# [r/SaaS](https://old.reddit.com/r/SaaS)
* [advertise](https://www.reddit.com/advertising)
1. **No self-promotion or advertising.** Posts promoting your product will be removed.
2. **Be helpful.** Answer the question that was asked.
Share your product in the [Weekly Feedback Thread](https://old.reddit.com/r/SaaS/x) every Monday.
Accounts must be 30 days old with 50 karma to post.`

func TestSubredditRulesFetcherTreg(t *testing.T) {
	var calls atomic.Int32
	var body map[string]string
	var path string
	tregSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		b, _ := json.Marshal(map[string]any{"output": map[string]string{"markdown": oldRedditRulesMD}})
		_, _ = w.Write(b)
	}))
	defer tregSrv.Close()
	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer blocked.Close()
	tc := treg.New(treg.Config{Token: "tok", BaseURL: tregSrv.URL}, nil, zerolog.Nop())

	// Production: no Reddit OAuth, treg on: straight to treg, Reddit not touched.
	var redditHits atomic.Int32
	reddit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redditHits.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer reddit.Close()
	fetch := SubredditRulesFetcher(&PublicSources{Client: reddit.Client(), RedditBase: reddit.URL}, tc)
	raw, err := fetch(context.Background(), tregTestWS, "SaaS")
	if err != nil || redditHits.Load() != 0 {
		t.Fatalf("err=%v redditHits=%d", err, redditHits.Load())
	}
	if path != "/call/"+tregPageTool || body["url"] != "https://old.reddit.com/r/SaaS/about/rules" || body["format"] != "md" {
		t.Fatalf("treg request: path=%q body=%v", path, body)
	}
	s := subrules.Summarize("SaaS", raw)
	if s.SelfPromo != subrules.PromoLimited || s.PromoThread != "Weekly Feedback Thread" || s.MinKarma != 50 || s.MinAccountAge != "30 days" {
		t.Fatalf("summary from scraped page: %+v", s)
	}
	if len(s.Quotes) == 0 || s.Quotes[0] != "No self-promotion or advertising. Posts promoting your product will be removed." {
		t.Fatalf("quotes = %q", s.Quotes)
	}

	// Reddit OAuth configured but refused: falls back to treg.
	oauth := &PublicSources{Client: blocked.Client(), RedditBase: blocked.URL,
		RedditOAuth: NewRedditOAuth("cid", "secret", "ua", blocked.URL, blocked.URL)}
	before := calls.Load()
	if _, err := SubredditRulesFetcher(oauth, tc)(context.Background(), tregTestWS, "SaaS"); err != nil || calls.Load() != before+1 {
		t.Fatalf("fallback: err=%v treg calls=%d", err, calls.Load()-before)
	}

	// No treg: the public path's error stands.
	if _, err := SubredditRulesFetcher(&PublicSources{Client: blocked.Client(), RedditBase: blocked.URL}, nil)(context.Background(), tregTestWS, "SaaS"); err == nil {
		t.Fatal("blocked Reddit without treg must fail")
	}
}

func TestTregSubredditRulesEmptyPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"markdown":""}`))
	}))
	defer srv.Close()
	tc := treg.New(treg.Config{Token: "tok", BaseURL: srv.URL}, nil, zerolog.Nop())
	if _, err := tregSubredditRules(context.Background(), tc, tregTestWS, "SaaS"); err == nil {
		t.Fatal("an empty page must be an error, so it gets the short failure cache")
	}
}

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
