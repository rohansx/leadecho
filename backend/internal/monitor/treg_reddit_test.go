package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

// Row shape captured live from crawl4ai.reddit.search.posts (2026-11-05).
const tregRedditBody = `{"meta":{"partial":false},"recipe":"reddit-search","rows":[
 {"author":"FugatoDev","created":"2026-11-05T08:35:49Z","id":"1xodhsh","link":"https://v.redd.it/x","score":null,
  "subreddit":"codex","text":"Codex can work in a Git worktree, is there an MCP server for this?","title":"I open sourced Fugato",
  "url":"https://www.reddit.com/r/codex/comments/1xodhsh/i_open_sourced_fugato/"},
 {"author":"linkposter","created":"2026-11-05T08:00:00Z","id":"1xlink","subreddit":"mcp","text":null,
  "title":"Best MCP server for Postgres?","url":"https://www.reddit.com/r/mcp/comments/1xlink/best/"}]}`

func TestParseTregPostsRedditRows(t *testing.T) {
	got := parseTregPosts([]byte(tregRedditBody))
	if len(got) != 2 {
		t.Fatalf("got %d posts %+v, want 2", len(got), got)
	}
	p := got[0]
	if p.ID != "1xodhsh" || p.Title != "I open sourced Fugato" || p.Subreddit != "codex" || p.Author != "FugatoDev" ||
		p.URL != "https://www.reddit.com/r/codex/comments/1xodhsh/i_open_sourced_fugato/" {
		t.Fatalf("post = %+v", p)
	}
	if want := time.Date(2026, 11, 5, 8, 35, 49, 0, time.UTC); !p.CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %v, want %v (the 'created' key)", p.CreatedAt, want)
	}
	// Link posts have no body: the title is the text, so they are not dropped.
	if got[1].Text != "Best MCP server for Postgres?" {
		t.Errorf("title-only post Text = %q", got[1].Text)
	}
}

func TestTregRedditParams(t *testing.T) {
	kw := database.ListActiveKeywordsRow{ID: "8a3c3b7e-8c1d-4c9e-9f1a-1b2c3d4e5f60", Term: "MCP server"}
	p := parseTregPosts([]byte(tregRedditBody))[0]
	params, ok := tregRedditParams(tregTestWS, kw, p)
	if !ok {
		t.Fatal("want ok")
	}
	if params.Platform != string(database.PlatformTypeReddit) || params.PlatformID != "reddit_1xodhsh" {
		t.Errorf("platform=%s id=%s (must match the other Reddit crawlers to dedupe)", params.Platform, params.PlatformID)
	}
	if params.Title.String != "I open sourced Fugato" || params.AuthorProfileUrl.String != "https://reddit.com/u/FugatoDev" {
		t.Errorf("title=%q author_url=%q", params.Title.String, params.AuthorProfileUrl.String)
	}
	var meta map[string]any
	_ = json.Unmarshal(params.PlatformMetadata, &meta)
	if meta["subreddit"] != "codex" || meta["source"] != "treg" {
		t.Errorf("metadata = %v", meta)
	}
	if _, ok := tregRedditParams(tregTestWS, kw, tregPost{Text: "no id"}); ok {
		t.Error("post without id must be skipped")
	}
}

func TestCrawlRedditTregRequest(t *testing.T) {
	var path string
	var body map[string]any
	m := tregMonitor(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"rows":[]}`))
	})
	if _, ok := m.crawlRedditTreg(context.Background(), tregTestWS, database.ListActiveKeywordsRow{Term: "MCP server"}); !ok {
		t.Fatal("answered miss must be ok")
	}
	if path != "/call/"+tregRedditTool || body["query"] != "MCP server" || body["sort"] != "relevance" || body["timeframe"] != "week" {
		t.Fatalf("path=%s body=%v", path, body)
	}
}

// Official Reddit OAuth (free) wins over treg; treg wins over the
// unauthenticated public crawler.
func TestRedditViaTreg(t *testing.T) {
	off := &Monitor{logger: zerolog.Nop(), public: &PublicSources{}}
	if off.redditViaTreg() {
		t.Error("treg disabled: must not use treg")
	}
	on := tregMonitor(t, func(http.ResponseWriter, *http.Request) {})
	on.public = &PublicSources{}
	if !on.redditViaTreg() {
		t.Error("treg enabled, no OAuth: must use treg")
	}
	on.public = &PublicSources{RedditOAuth: &RedditOAuth{}}
	if on.redditViaTreg() {
		t.Error("Reddit OAuth configured: must prefer the free official API")
	}
}
