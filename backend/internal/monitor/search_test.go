package monitor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestSources(t *testing.T, h http.HandlerFunc) *PublicSources {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &PublicSources{Client: srv.Client(), RedditBase: srv.URL, HNBase: srv.URL, DevToBase: srv.URL}
}

func TestSearchHackerNews(t *testing.T) {
	now := time.Now()
	var gotQuery string
	src := newTestSources(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprintf(w, `{"hits":[
			{"objectID":"1","title":"Need a CRM","story_text":"<p>any &#x27;good&#x27; one?","author":"a","points":3,"created_at_i":%d},
			{"objectID":"2","comment_text":"we use <i>Foo</i> for this","author":"b","created_at_i":%d},
			{"objectID":"3","author":"c","created_at_i":%d}
		]}`, now.Unix(), now.Unix(), now.Unix())
	})

	since := now.Add(-7 * 24 * time.Hour)
	posts, err := src.SearchHackerNews(context.Background(), "crm tool", since)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "query=crm+tool") || !strings.Contains(gotQuery, fmt.Sprintf("created_at_i%%3E%d", since.Unix())) {
		t.Fatalf("query missing term or since filter: %s", gotQuery)
	}
	if len(posts) != 2 {
		t.Fatalf("want 2 posts (empty one skipped), got %d", len(posts))
	}
	if posts[0].Content != "Need a CRM\n\nany 'good' one?" || posts[0].URL != "https://news.ycombinator.com/item?id=1" || posts[0].ExternalID != "hn_1" {
		t.Fatalf("unexpected first post: %+v", posts[0])
	}
	if posts[1].Content != "we use Foo for this" {
		t.Fatalf("comment text not cleaned: %q", posts[1].Content)
	}
}

func TestSearchRedditAndSinceFilter(t *testing.T) {
	now := time.Now()
	old := now.Add(-30 * 24 * time.Hour)
	var gotPath, gotQuery string
	src := newTestSources(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		fmt.Fprintf(w, `{"data":{"children":[
			{"data":{"id":"x1","title":"Looking for a CRM","selftext":"budget 50","author":"u1","subreddit":"saas","permalink":"/r/saas/x1","created_utc":%d}},
			{"data":{"id":"x2","title":"old","author":"u2","permalink":"/r/saas/x2","created_utc":%d}},
			{"data":{"id":"x3","title":"bot","author":"AutoModerator","permalink":"/r/saas/x3","created_utc":%d}}
		]}}`, now.Unix(), old.Unix(), now.Unix())
	})

	posts, err := src.SearchReddit(context.Background(), "crm", now.Add(-7*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/search.json" || !strings.Contains(gotQuery, "t=week") || !strings.Contains(gotQuery, "q=crm") {
		t.Fatalf("bad request %s?%s", gotPath, gotQuery)
	}
	if len(posts) != 1 || posts[0].URL != "https://reddit.com/r/saas/x1" || posts[0].Content != "Looking for a CRM\n\nbudget 50" {
		t.Fatalf("unexpected posts: %+v", posts)
	}

	// Subreddit listing without since keeps the old post (existing behaviour).
	all, err := src.SubredditNew(context.Background(), "saas", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/r/saas/new.json" || len(all) != 2 {
		t.Fatalf("subreddit listing: path=%s posts=%d", gotPath, len(all))
	}
}

func TestRedditStatusErrors(t *testing.T) {
	for _, code := range []int{403, 429, 500} {
		src := newTestSources(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) })
		_, err := src.SearchReddit(context.Background(), "crm", time.Time{})
		var se *StatusError
		if !errors.As(err, &se) || se.Code != code {
			t.Fatalf("code %d: want StatusError, got %v", code, err)
		}
	}
}

func TestSearchDevTo(t *testing.T) {
	now := time.Now().UTC()
	var gotQuery string
	src := newTestSources(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprintf(w, `[
			{"id":7,"title":"Postgres tips","description":"some long enough description","url":"https://dev.to/a/p","published_at":%q,"user":{"username":"a"}},
			{"id":8,"title":"Old","url":"https://dev.to/a/o","published_at":%q,"user":{"username":"b"}}
		]`, now.Format(time.RFC3339), now.AddDate(0, -2, 0).Format(time.RFC3339))
	})
	posts, err := src.SearchDevTo(context.Background(), "postgres tuning", now.Add(-7*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "tag=postgres") {
		t.Fatalf("tag not derived from first word: %s", gotQuery)
	}
	if len(posts) != 1 || posts[0].ExternalID != "devto_7" {
		t.Fatalf("unexpected posts %+v", posts)
	}
	// No usable word -> no request, no posts.
	if p, err := src.SearchDevTo(context.Background(), "a 12", time.Time{}); err != nil || len(p) != 0 {
		t.Fatalf("want empty, got %v %v", p, err)
	}
}

func TestSearchDispatch(t *testing.T) {
	src := newTestSources(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"hits":[]}`) })
	if _, err := src.Search(context.Background(), "hackernews", "x", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Search(context.Background(), "myspace", "x", time.Time{}); err == nil {
		t.Fatal("unsupported platform must error")
	}
}

func TestSinceContext(t *testing.T) {
	ctx := context.Background()
	if !sinceFrom(ctx).IsZero() {
		t.Fatal("default since must be zero")
	}
	ts := time.Now().Add(-time.Hour)
	if got := sinceFrom(WithSince(ctx, ts)); !got.Equal(ts) {
		t.Fatalf("got %v", got)
	}
}

func TestRedditTimeWindow(t *testing.T) {
	now := time.Now()
	cases := []struct {
		since time.Time
		want  string
	}{
		{time.Time{}, "week"},
		{now.Add(-12 * time.Hour), "day"},
		{now.Add(-6 * 24 * time.Hour), "week"},
		{now.Add(-20 * 24 * time.Hour), "month"},
		{now.Add(-90 * 24 * time.Hour), "year"},
	}
	for _, c := range cases {
		if got := redditTimeWindow(c.since); got != c.want {
			t.Errorf("since %v: got %s want %s", c.since, got, c.want)
		}
	}
}
