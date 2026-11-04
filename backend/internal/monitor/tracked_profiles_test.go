package monitor

import (
	"strings"
	"testing"
	"time"

	"leadecho/internal/database"
)

func TestTrackedRequest(t *testing.T) {
	cases := []struct {
		platform, handle, wantTool, wantKey string
	}{
		{"linkedin", "williamhgates", tregLinkedInUserPostsTool, "profile"},
		{"x", "levelsio", tregXUserPostsTool, "username"},
	}
	for _, c := range cases {
		tool, body := trackedRequest(database.TrackedProfile{Platform: c.platform, Handle: c.handle})
		if tool != c.wantTool || body[c.wantKey] != c.handle {
			t.Errorf("%s: tool=%s body=%v", c.platform, tool, body)
		}
	}
}

func TestParseTrackedPayloads(t *testing.T) {
	cases := []struct {
		name, body string
		wantText   string
		wantTime   time.Time
	}{
		{"crawl4ai linkedin rows",
			`{"rows":[{"text":"Hiring a RevOps lead","url":"https://www.linkedin.com/feed/update/urn:li:activity:7514893923710984192/","posted_at":"2026-10-11T03:45:31Z","likes":92}]}`,
			"Hiring a RevOps lead", time.Date(2026, 10, 11, 3, 45, 31, 0, time.UTC)},
		{"anyapi x tweets under output.data",
			`{"output":{"data":{"tweets":[{"id":"2101118004087886212","createdUtc":1789780543,"text":"shipping today","url":"https://x.com/i/web/status/2101118004087886212"}]}}}`,
			"shipping today", time.Unix(1789780543, 0).UTC()},
	}
	for _, c := range cases {
		posts := parseTregPosts([]byte(c.body))
		if len(posts) != 1 || posts[0].Text != c.wantText || !posts[0].CreatedAt.Equal(c.wantTime) {
			t.Errorf("%s: got %+v", c.name, posts)
		}
	}
}

func TestTrackedMentionParams(t *testing.T) {
	created := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	x := database.TrackedProfile{ID: "tp1", WorkspaceID: "ws", Platform: "x", Handle: "levelsio", CreatedAt: created}
	li := database.TrackedProfile{ID: "tp2", WorkspaceID: "ws", Platform: "linkedin", Handle: "jane", CreatedAt: created}
	recent := created.Add(-time.Hour)
	cases := []struct {
		name           string
		prof           database.TrackedProfile
		post           tregPost
		ok             bool
		wantPlatformID string
		wantURL        string
	}{
		{"x post gets canonical url and twitter_ id", x, tregPost{ID: "42", Text: "hi", URL: "https://x.com/i/web/status/42", CreatedAt: recent}, true, "twitter_42", "https://x.com/levelsio/status/42"},
		{"x id from url", x, tregPost{Text: "hi", URL: "https://x.com/levelsio/status/43"}, true, "twitter_43", "https://x.com/levelsio/status/43"},
		{"linkedin activity id", li, tregPost{Text: "post", URL: "https://www.linkedin.com/feed/update/urn:li:activity:777/", CreatedAt: recent}, true, "linkedin_777", "https://www.linkedin.com/feed/update/urn:li:activity:777/"},
		{"old post before tracking is skipped", li, tregPost{Text: "old", URL: "https://www.linkedin.com/feed/update/urn:li:activity:1/", CreatedAt: created.AddDate(0, -2, 0)}, false, "", ""},
		{"no id is skipped", x, tregPost{Text: "hi"}, false, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, ok := trackedMentionParams(c.prof, c.post)
			if ok != c.ok {
				t.Fatalf("ok = %v", ok)
			}
			if !ok {
				return
			}
			if p.PlatformID != c.wantPlatformID || p.Url != c.wantURL || p.WorkspaceID != "ws" {
				t.Errorf("params = %+v", p)
			}
			if p.AuthorUsername.String != c.prof.Handle || p.KeywordMatches == nil || !strings.Contains(string(p.PlatformMetadata), `"tracked_profile"`) {
				t.Errorf("author/meta = %q %s", p.AuthorUsername.String, p.PlatformMetadata)
			}
		})
	}
}
