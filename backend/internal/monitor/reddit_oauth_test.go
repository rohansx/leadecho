package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRedditOAuthSearch(t *testing.T) {
	var tokenCalls, searchCalls atomic.Int32
	var expireFirst atomic.Bool
	expireFirst.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/access_token":
			tokenCalls.Add(1)
			if u, p, ok := r.BasicAuth(); !ok || u != "cid" || p != "secret" || r.FormValue("grant_type") != "client_credentials" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tok` + strconv.Itoa(int(tokenCalls.Load())) + `","expires_in":3600}`))
		case "/search":
			searchCalls.Add(1)
			// The first token is rejected once to exercise the refresh-on-401 path.
			if r.Header.Get("Authorization") == "Bearer tok1" && expireFirst.Swap(false) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer tok") || !strings.Contains(r.UserAgent(), "leadecho") {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"children":[{"data":{"id":"abc","title":"Need a CRM","author":"jane","subreddit":"saas","permalink":"/r/saas/comments/abc/x/","created_utc":` + strconv.FormatInt(time.Now().Unix(), 10) + `}}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	s := &PublicSources{Client: srv.Client(), RedditBase: "https://www.reddit.com",
		RedditOAuth: NewRedditOAuth("cid", "secret", "web:leadecho:1.0", srv.URL, srv.URL)}
	for i := 0; i < 2; i++ {
		posts, err := s.SearchReddit(context.Background(), "crm", time.Time{})
		if err != nil || len(posts) != 1 || posts[0].ExternalID != "reddit_abc" {
			t.Fatalf("search %d: posts=%v err=%v", i, posts, err)
		}
	}
	if tokenCalls.Load() != 2 { // initial + one refresh after the 401, then cached
		t.Fatalf("token calls = %d, want 2", tokenCalls.Load())
	}
	if searchCalls.Load() != 3 {
		t.Fatalf("search calls = %d, want 3", searchCalls.Load())
	}
}

func TestRedditOAuthFromEnv(t *testing.T) {
	t.Setenv("REDDIT_CLIENT_ID", "")
	if RedditOAuthFromEnv() != nil {
		t.Fatal("no credentials -> nil (public JSON path)")
	}
	t.Setenv("REDDIT_CLIENT_ID", "cid")
	t.Setenv("REDDIT_CLIENT_SECRET", "s")
	if RedditOAuthFromEnv() == nil {
		t.Fatal("credentials set -> oauth enabled")
	}
}
