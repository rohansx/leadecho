package monitor

import (
	"encoding/json"
	"testing"
	"time"

	"leadecho/internal/database"
)

func TestParseTregPosts(t *testing.T) {
	ts := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		body string
		want []tregPost
	}{
		{
			name: "routed envelope, anyapi-style rows",
			body: `{"output":{"posts":[{"id":"1850","text":"anyone know a good CRM for startups?","url":"https://x.com/ana/status/1850","createdAt":"2026-10-30T12:00:00Z","likeCount":4,"replyCount":2,"retweetCount":1,"author":{"userName":"ana","name":"Ana"}}]},"_treg":{"served_by":"anyapi"}}`,
			want: []tregPost{{ID: "1850", Text: "anyone know a good CRM for startups?", URL: "https://x.com/ana/status/1850", Author: "ana", CreatedAt: ts, Likes: 4, Replies: 2, Reposts: 1}},
		},
		{
			name: "twitter v1 shape with legacy, numeric id and ruby date",
			body: `{"output":{"posts":[{"legacy":{"id_str":"99","full_text":"looking for an alternative to hubspot","created_at":"Fri Oct 30 12:00:00 +0000 2026","favorite_count":7},"user":{"screen_name":"bob"}}]}}`,
			want: []tregPost{{ID: "99", Text: "looking for an alternative to hubspot", Author: "bob", CreatedAt: ts, Likes: 7}},
		},
		{
			name: "direct provider body, linkedin rows under results",
			body: `{"results":[{"text":"we are evaluating sales tools","link":"https://www.linkedin.com/feed/update/urn:li:activity:7123","publishDate":"2026-10-30","author":{"name":"Cara Doe","url":"https://www.linkedin.com/in/cara"}}]}`,
			want: []tregPost{{Text: "we are evaluating sales tools", URL: "https://www.linkedin.com/feed/update/urn:li:activity:7123", Author: "Cara Doe", AuthorURL: "https://www.linkedin.com/in/cara", CreatedAt: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)}},
		},
		{
			name: "unix seconds and string author",
			body: `{"data":[{"id":5,"content":"hello there friends","author":"dan","timestamp":1793361600}]}`,
			want: []tregPost{{ID: "5", Text: "hello there friends", Author: "dan", CreatedAt: ts}},
		},
		{"miss", `{"output":{"posts":[]}}`, nil},
		{"garbage", `not json`, nil},
		{"rows without text dropped", `{"output":{"posts":[{"id":"1"}]}}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTregPosts([]byte(tc.body))
			if len(got) != len(tc.want) {
				t.Fatalf("got %d posts %+v, want %d", len(got), got, len(tc.want))
			}
			for i := range got {
				if !got[i].CreatedAt.Equal(tc.want[i].CreatedAt) {
					t.Errorf("CreatedAt = %v, want %v", got[i].CreatedAt, tc.want[i].CreatedAt)
				}
				got[i].CreatedAt, tc.want[i].CreatedAt = time.Time{}, time.Time{}
				if got[i] != tc.want[i] {
					t.Errorf("post = %+v\nwant  %+v", got[i], tc.want[i])
				}
			}
		})
	}
}

func TestTregTwitterParams(t *testing.T) {
	kw := database.ListActiveKeywordsRow{ID: "8a3c3b7e-8c1d-4c9e-9f1a-1b2c3d4e5f60", Term: "crm"}
	cases := []struct {
		name    string
		post    tregPost
		ok      bool
		wantID  string
		wantURL string
	}{
		{"full", tregPost{ID: "1", Text: "t", URL: "https://x.com/a/status/1", Author: "a"}, true, "twitter_1", "https://x.com/a/status/1"},
		{"id from url", tregPost{Text: "t", URL: "https://x.com/a/status/42?s=1"}, true, "twitter_42", "https://x.com/a/status/42?s=1"},
		{"url built from author+id", tregPost{ID: "7", Text: "t", Author: "@zed"}, true, "twitter_7", "https://x.com/zed/status/7"},
		{"no id", tregPost{Text: "t", Author: "a"}, false, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := tregTwitterParams("ws", kw, tc.post)
			if ok != tc.ok {
				t.Fatalf("ok = %v", ok)
			}
			if !ok {
				return
			}
			if p.PlatformID != tc.wantID || p.Url != tc.wantURL || p.Platform != string(database.PlatformTypeTwitter) || p.WorkspaceID != "ws" || !p.KeywordID.Valid {
				t.Fatalf("params = %+v", p)
			}
			if p.AuthorUsername.Valid && p.AuthorProfileUrl.String != "https://x.com/"+p.AuthorUsername.String {
				t.Fatalf("profile url = %s", p.AuthorProfileUrl.String)
			}
			var meta map[string]any
			_ = json.Unmarshal(p.PlatformMetadata, &meta)
			if meta["source"] != "treg" {
				t.Fatalf("meta = %v", meta)
			}
		})
	}
}

func TestTregLinkedInParams(t *testing.T) {
	kw := database.ListActiveKeywordsRow{ID: "8a3c3b7e-8c1d-4c9e-9f1a-1b2c3d4e5f60", Term: "crm"}
	cases := []struct {
		name   string
		post   tregPost
		ok     bool
		wantID string
	}{
		{"urn in url", tregPost{Text: "t", URL: "https://www.linkedin.com/feed/update/urn:li:activity:7123/"}, true, "linkedin_7123"},
		{"urn in id", tregPost{ID: "urn:li:activity:55", Text: "t", URL: "https://lnkd.in/x"}, true, "linkedin_55"},
		{"plain id", tregPost{ID: "abc", Text: "t", URL: "https://www.linkedin.com/posts/foo"}, true, "linkedin_abc"},
		{"no url", tregPost{ID: "1", Text: "t"}, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := tregLinkedInParams("ws", kw, tc.post)
			if ok != tc.ok {
				t.Fatalf("ok = %v", ok)
			}
			if ok && (p.PlatformID != tc.wantID || p.Platform != string(database.PlatformTypeLinkedin)) {
				t.Fatalf("params = %+v", p)
			}
		})
	}
}
