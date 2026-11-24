package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/billing"
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
			for _, crawl := range []func(context.Context, string, database.ListActiveKeywordsRow) ([]mentionAlert, error){m.crawlTwitterTreg, m.crawlLinkedInTreg} {
				alerts, err := crawl(context.Background(), tregTestWS, kw)
				if ok := err == nil; ok != tc.wantOK || len(alerts) != 0 {
					t.Fatalf("ok=%v alerts=%d, want ok=%v", ok, len(alerts), tc.wantOK)
				}
			}
		})
	}
}

// Only a search treg refused for now (our balance, a cap) stays due. A provider
// error is stamped as before (the provider may have been paid), and Reddit
// always counts as scanned: its public fallback ran instead.
func TestCrawlKeywordReportsFailedPaidPlatforms(t *testing.T) {
	kw := cursorKW
	kw.Platforms = []string{"twitter", "linkedin", "reddit"} // no subreddits: the public fallback returns at once
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   []string
	}{
		{"balance empty", 402, `{"detail":{"error":"insufficient_balance"}}`, []string{"twitter", "linkedin"}},
		{"workspace cap", 429, `{"detail":{"error":"tag_spend_cap_reached"}}`, []string{"twitter", "linkedin"}},
		{"provider 4xx", 400, `{}`, nil},
		{"provider 429", 429, `{}`, nil},
		{"provider 5xx", 502, `{"error":"route_failed"}`, nil},
		{"answered", 200, `{"output":{"posts":[]}}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := tregMonitor(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			_, failed := m.crawlKeyword(context.Background(), tregTestWS, kw)
			if !slices.Equal(failed, tc.want) {
				t.Fatalf("failed=%q, want %q", failed, tc.want)
			}
		})
	}
}

func TestRetryLater(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{treg.ErrInsufficientBalance, true},
		{treg.ErrPlatformDailyCap, true},
		{treg.ErrPaused, true},
		{treg.ErrCustomerDailyCap, true},
		{&treg.APIError{Status: 400, Code: "http_400"}, false},
		{&treg.APIError{Status: 502, Code: "route_failed"}, false},
		{context.DeadlineExceeded, false},
		{billing.ErrPaidDataPaused, false},
		{billing.ErrNoCredits, false},
		{ErrTregOff, false},
		{nil, false},
	} {
		if got := retryLater(tc.err); got != tc.want {
			t.Errorf("retryLater(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

// A refused platform is not stamped and waits retryGap; the others are
// stamped; a later success clears the wait.
func TestRecordScansRetryGap(t *testing.T) {
	m := &Monitor{logger: zerolog.Nop()} // no DB: the stamp itself is skipped
	kw := cursorKW
	kw.Platforms = []string{"twitter", "linkedin", "hackernews"}
	m.recordScans(context.Background(), tregTestWS, kw, []string{"twitter"})

	due := m.withoutRetryWait(tregTestWS, []database.ListActiveKeywordsRow{kw})
	if len(due) != 1 || !slices.Equal(due[0].Platforms, []string{"linkedin", "hackernews"}) {
		t.Fatalf("due=%+v, want twitter held back for retryGap", due)
	}
	if !slices.Equal(kw.Platforms, []string{"twitter", "linkedin", "hackernews"}) {
		t.Fatalf("input mutated: %q", kw.Platforms)
	}
	until, ok := m.scanRetry.Load(retryKey(tregTestWS, kw.ID, "twitter"))
	if !ok || time.Until(until.(time.Time)) < retryGap-time.Minute {
		t.Fatalf("retry wait = %v, want about %v", until, retryGap)
	}

	m.scanRetry.Store(retryKey(tregTestWS, kw.ID, "twitter"), time.Now().Add(-time.Second)) // gap over
	if due := m.withoutRetryWait(tregTestWS, []database.ListActiveKeywordsRow{kw}); len(due[0].Platforms) != 3 {
		t.Fatalf("after the gap twitter must be due again: %+v", due)
	}
	m.recordScans(context.Background(), tregTestWS, kw, nil)
	if _, held := m.scanRetry.Load(retryKey(tregTestWS, kw.ID, "twitter")); held {
		t.Fatal("a successful scan must clear the retry wait")
	}
}

func TestCrawlTregDisabled(t *testing.T) {
	m := &Monitor{logger: zerolog.Nop()}
	if _, err := m.crawlTwitterTreg(context.Background(), tregTestWS, database.ListActiveKeywordsRow{Term: "x"}); err == nil {
		t.Fatal("disabled treg must report ok=false")
	}
}

// memState is an in-memory treg.Cache (TTLs ignored).
type memState map[string]string

func (s memState) Get(_ context.Context, k string) ([]byte, bool) {
	v, ok := s[k]
	return []byte(v), ok
}
func (s memState) Set(_ context.Context, k string, v []byte, _ time.Duration) { s[k] = string(v) }

// Posts here never match the keyword, so nothing reaches the database.
var cursorKW = database.ListActiveKeywordsRow{ID: "8a3c3b7e-8c1d-4c9e-9f1a-1b2c3d4e5f60", Term: "zebra-unmatched-term", MatchType: "phrase"}

func TestCrawlTwitterTregAsksOnlyForNewerPosts(t *testing.T) {
	var queries, waterfall []string
	status, tregRefusal := 200, false
	m := tregMonitor(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		queries = append(queries, body["q"])
		waterfall = append(waterfall, r.Header.Get("X-Treg-Route-Waterfall"))
		if tregRefusal {
			w.Header().Set("X-Treg-Error", "1")
		}
		w.WriteHeader(status)
		if len(queries) == 1 {
			_, _ = w.Write([]byte(`{"output":{"posts":[{"id":"900","text":"an unrelated post"},{"id":"1000","text":"another unrelated post"}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"output":{"posts":[]}}`))
	})
	m.SetTregState(memState{})
	ctx := context.Background()

	for i := 0; i < 3; i++ { // full search, then two polls with nothing new
		if _, err := m.crawlTwitterTreg(ctx, tregTestWS, cursorKW); err != nil {
			t.Fatalf("poll %d: ok=false", i)
		}
	}
	cursorQ := "(zebra-unmatched-term) since_id:1000"
	if queries[0] != "zebra-unmatched-term" || queries[1] != cursorQ || queries[2] != cursorQ {
		t.Fatalf("queries=%q", queries)
	}
	if waterfall[0] != "" || waterfall[1] != "0" || waterfall[2] != "0" {
		t.Fatalf("waterfall headers=%q, want every cursor poll to send 0", waterfall)
	}

	// A treg-side refusal (cap, block, billing) keeps the cursor.
	status, tregRefusal = 403, true
	if _, err := m.crawlTwitterTreg(ctx, tregTestWS, cursorKW); err == nil {
		t.Fatal("refused poll must report ok=false")
	}
	status, tregRefusal = 200, false
	m.crawlTwitterTreg(ctx, tregTestWS, cursorKW)
	if got := queries[len(queries)-1]; got != cursorQ {
		t.Fatalf("after a treg refusal the query was %q, want the cursor kept", got)
	}

	// A provider rejecting the cursor query drops the cursor: the next poll is a plain search.
	status = 400
	if _, err := m.crawlTwitterTreg(ctx, tregTestWS, cursorKW); err == nil {
		t.Fatal("failed poll must report ok=false")
	}
	status = 200
	m.crawlTwitterTreg(ctx, tregTestWS, cursorKW)
	if got := queries[len(queries)-1]; got != "zebra-unmatched-term" {
		t.Fatalf("after a failed cursor poll the query was %q, want a plain search", got)
	}
}

func TestNewestTweetID(t *testing.T) {
	posts := []tregPost{{ID: "12"}, {ID: "urn:x", URL: "https://x.com/a/status/99"}, {ID: "not-a-number"}}
	if got := newestTweetID(posts, "50"); got != "99" {
		t.Fatalf("got %q, want 99", got)
	}
	if got := newestTweetID(posts, "500"); got != "500" {
		t.Fatalf("an older page must not move the cursor back: got %q", got)
	}
	if got := newestTweetID(nil, ""); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestCrawlLinkedInTregPollWindow(t *testing.T) {
	var windows []string
	status := 200
	m := tregMonitor(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		windows = append(windows, body["datePosted"])
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"output":{"posts":[]}}`))
	})
	state := memState{}
	m.SetTregState(state)
	ctx := context.Background()
	key := tregStateKey("li_polled", tregTestWS, cursorKW)

	// A failed call doesn't count as a poll: the next tick tries again.
	status = 400
	if _, err := m.crawlLinkedInTreg(ctx, tregTestWS, cursorKW); err == nil {
		t.Fatal("failed poll must report ok=false")
	}
	if _, polled := state[key]; polled {
		t.Fatal("a failed call must not record a poll")
	}
	status = 200
	windows = nil

	// First poll searches the week; a second one right after makes no call.
	for i := 0; i < 2; i++ {
		if _, err := m.crawlLinkedInTreg(ctx, tregTestWS, cursorKW); err != nil {
			t.Fatalf("poll %d: ok=false", i)
		}
	}
	if len(windows) != 1 || windows[0] != "last-week" {
		t.Fatalf("windows=%q, want one last-week call", windows)
	}

	// Once the poll interval has passed, only the past day is searched.
	state[key] = strconv.FormatInt(time.Now().Add(-linkedInPollEvery-time.Minute).Unix(), 10)
	m.crawlLinkedInTreg(ctx, tregTestWS, cursorKW)
	if len(windows) != 2 || windows[1] != "last-day" {
		t.Fatalf("windows=%q, want a last-day call after the interval", windows)
	}

	// Without state (no Redis) every poll is a full-week search, as before.
	m.SetTregState(nil)
	m.crawlLinkedInTreg(ctx, tregTestWS, cursorKW)
	m.crawlLinkedInTreg(ctx, tregTestWS, cursorKW)
	if len(windows) != 4 || windows[3] != "last-week" {
		t.Fatalf("windows=%q, want two more last-week calls", windows)
	}
}
