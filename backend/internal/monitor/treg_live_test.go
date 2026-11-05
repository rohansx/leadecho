//go:build live

// Live treg check (spends a few tenths of a cent). Run:
//
//	TREG_TOKEN=... TREG_LIVE_DUMP=/tmp/dir go test -tags live -run TestTregLive -v ./internal/monitor/
package monitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/treg"
)

func TestTregLive(t *testing.T) {
	token := os.Getenv("TREG_TOKEN")
	if token == "" {
		t.Skip("TREG_TOKEN not set")
	}
	c := treg.New(treg.Config{Token: token, OrgID: os.Getenv("TREG_ORG_ID"), MaxCostMicro: 20000}, nil, zerolog.Nop())

	cases := []struct {
		name, tool string
		body       any
	}{
		{"x", tregXTool, map[string]string{"q": "MCP server"}},
		{"linkedin", tregLinkedInTool, map[string]string{"query": "MCP server", "datePosted": "last-week"}},
		{"reddit", "crawl4ai.reddit.search.posts", map[string]any{"query": "MCP server", "sort": "new", "t": "week", "limit": 25}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			resp, err := c.Call(ctx, treg.Request{WorkspaceID: "live-test", Tool: tc.tool, Body: tc.body})
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if dir := os.Getenv("TREG_LIVE_DUMP"); dir != "" {
				_ = os.WriteFile(filepath.Join(dir, "treg-"+tc.name+".json"), resp.Body, 0o600)
			}
			posts := parseTregPosts(resp.Body)
			t.Logf("status=%d cost_micro=%d call_id=%s served_by=%s posts=%d", resp.Status, resp.CostMicro, resp.CallID, resp.ServedBy, len(posts))
			if len(posts) == 0 {
				t.Fatalf("parsed 0 posts from %d-byte body", len(resp.Body))
			}
			for i, p := range posts[:min(3, len(posts))] {
				t.Logf("#%d id=%q url=%q author=%q created=%s text=%.80q", i, p.ID, p.URL, p.Author, p.CreatedAt.Format(time.RFC3339), p.Text)
				if p.Text == "" || p.URL == "" {
					t.Errorf("post %d missing text or url: %+v", i, p)
				}
			}
		})
	}
}
