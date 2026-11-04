package monitor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
)

const bskySearchBody = `{"posts":[{
	"uri":"at://did:plc:abc/app.bsky.feed.post/3kxyz",
	"author":{"did":"did:plc:abc","handle":"alice.bsky.social","displayName":"Alice"},
	"record":{"$type":"app.bsky.feed.post","text":"Any good CRM alternative?","createdAt":"2026-11-04T09:15:07+00:00"},
	"likeCount":3,"replyCount":1,"repostCount":2
}]}`

func withBlueskyBases(t *testing.T, public, auth string) {
	t.Helper()
	op, oa := blueskyPublicBase, blueskyAuthBase
	blueskyPublicBase, blueskyAuthBase = public, auth
	t.Cleanup(func() { blueskyPublicBase, blueskyAuthBase = op, oa })
}

func TestFetchBlueskyUnauthenticated(t *testing.T) {
	var gotQ, gotLimit, gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQ, gotLimit, gotAuth = r.URL.Path, r.URL.Query().Get("q"), r.URL.Query().Get("limit"), r.Header.Get("Authorization")
		io.WriteString(w, bskySearchBody)
	}))
	defer srv.Close()
	withBlueskyBases(t, srv.URL, "http://unused.invalid")

	m := &Monitor{logger: zerolog.Nop()}
	posts, err := m.fetchBluesky(context.Background(), "crm alternative")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/app.bsky.feed.searchPosts" || gotQ != "crm alternative" || gotLimit != "25" {
		t.Errorf("request path=%q q=%q limit=%q", gotPath, gotQ, gotLimit)
	}
	if gotAuth != "" {
		t.Errorf("unauthenticated request sent Authorization %q", gotAuth)
	}
	if len(posts) != 1 || posts[0].Author.Handle != "alice.bsky.social" || posts[0].Record.Text != "Any good CRM alternative?" || posts[0].LikeCount != 3 {
		t.Fatalf("parsed = %+v", posts)
	}
}

func TestFetchBlueskyWithSession(t *testing.T) {
	var sessions atomic.Int32
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/com.atproto.server.createSession":
			sessions.Add(1)
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.Method != http.MethodPost || body["identifier"] != "me.bsky.social" || body["password"] != "app-pw" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			io.WriteString(w, `{"accessJwt":"jwt-1","refreshJwt":"r"}`)
		case "/app.bsky.feed.searchPosts":
			gotAuth = r.Header.Get("Authorization")
			io.WriteString(w, bskySearchBody)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	withBlueskyBases(t, "http://unused.invalid", srv.URL)

	m := &Monitor{logger: zerolog.Nop()}
	m.SetBlueskyCredentials("me.bsky.social", "app-pw")
	for i := 0; i < 2; i++ {
		if _, err := m.fetchBluesky(context.Background(), "crm"); err != nil {
			t.Fatal(err)
		}
	}
	if gotAuth != "Bearer jwt-1" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if n := sessions.Load(); n != 1 {
		t.Errorf("createSession called %d times, want 1 (cached)", n)
	}
}

func TestFetchBlueskyErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	withBlueskyBases(t, srv.URL, srv.URL)

	m := &Monitor{logger: zerolog.Nop()}
	if _, err := m.fetchBluesky(context.Background(), "x"); err == nil {
		t.Error("want error on 403 search")
	}
	m.SetBlueskyCredentials("me", "bad")
	if _, err := m.fetchBluesky(context.Background(), "x"); err == nil {
		t.Error("want error on failed createSession")
	}
}

func TestBlueskyPostURL(t *testing.T) {
	tests := []struct{ uri, handle, want string }{
		{"at://did:plc:abc/app.bsky.feed.post/3kxyz", "alice.bsky.social", "https://bsky.app/profile/alice.bsky.social/post/3kxyz"},
		{"at://did:plc:abc/app.bsky.feed.post/3kxyz", "", "https://bsky.app/profile/did:plc:abc/post/3kxyz"},
		{"garbage", "alice", ""},
		{"at://did:plc:abc/app.bsky.feed.like/1", "alice", ""},
	}
	for _, tt := range tests {
		if got := blueskyPostURL(tt.uri, tt.handle); got != tt.want {
			t.Errorf("blueskyPostURL(%q,%q) = %q, want %q", tt.uri, tt.handle, got, tt.want)
		}
	}
}
