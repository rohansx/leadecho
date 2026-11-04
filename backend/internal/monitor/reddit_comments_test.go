package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

const commentsFixture = `{"data":{"children":[
 {"kind":"t1","data":{"id":"c1","body":"We switched off HubSpot, anyone tried a cheaper CRM?","author":"alice","subreddit":"sales","permalink":"/r/sales/comments/p1/t/c1/","link_title":"CRM thread","link_id":"t3_p1","score":5,"created_utc":1793361600}},
 {"kind":"t1","data":{"id":"c2","body":"[deleted]","author":"[deleted]","permalink":"/r/sales/comments/p1/t/c2/"}},
 {"kind":"t1","data":{"id":"c3","body":"AutoModerator says hi to everyone","author":"AutoModerator","permalink":"/x"}}
]}}`

func TestFetchSubredditComments(t *testing.T) {
	var path, ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, ua = r.URL.Path, r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(commentsFixture))
	}))
	defer srv.Close()
	m := &Monitor{logger: zerolog.Nop()}
	comments, err := m.fetchSubredditComments(context.Background(), srv.URL, "sales")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/r/sales/comments.json" || ua == "" {
		t.Fatalf("path=%s ua=%q", path, ua)
	}
	if len(comments) != 1 || comments[0].ID != "c1" {
		t.Fatalf("comments = %+v (deleted/automod must be dropped)", comments)
	}
}

func TestFetchSubredditCommentsBackoff(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusForbidden} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		m := &Monitor{logger: zerolog.Nop()}
		if _, err := m.fetchSubredditComments(context.Background(), srv.URL, "sales"); err == nil {
			t.Fatalf("%d: want error", status)
		}
		if !m.redditBackedOff() {
			t.Fatalf("%d: must set reddit backoff", status)
		}
		srv.Close()
	}
}

func TestRedditCommentParams(t *testing.T) {
	kw := database.ListActiveKeywordsRow{ID: "8a3c3b7e-8c1d-4c9e-9f1a-1b2c3d4e5f60", Term: "crm"}
	c := redditComment{ID: "c1", Body: "body", Author: "alice", Subreddit: "sales", Permalink: "/r/sales/comments/p1/t/c1/", LinkTitle: "CRM thread", LinkID: "t3_p1", Score: 5, CreatedUTC: 1793361600}
	p := redditCommentParams("ws", kw, c)
	if p.PlatformID != "reddit_comment_c1" || p.Url != "https://reddit.com/r/sales/comments/p1/t/c1/" || p.Platform != "reddit" {
		t.Fatalf("params = %+v", p)
	}
	if p.Title.String != "CRM thread" || p.AuthorUsername.String != "alice" || p.Content != "body" || !p.PlatformCreatedAt.Valid {
		t.Fatalf("params = %+v", p)
	}
}

func TestValidSubreddit(t *testing.T) {
	for in, want := range map[string]bool{"sales": true, "Start_Ups": true, "": false, "a/b": false, "../x": false, "a?b": false} {
		if got := validSubreddit(in); got != want {
			t.Errorf("validSubreddit(%q) = %v", in, got)
		}
	}
}
