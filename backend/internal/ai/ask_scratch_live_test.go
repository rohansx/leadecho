//go:build live

// Scratch check (not committed): what the current "ask" rule extracts from
// real posts. ASK_SAMPLE is a JSON array of {platform,title,content,old_ask}.
package ai

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"
)

func TestAskRuleOnRealPosts(t *testing.T) {
	key, path, model := os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("ASK_SAMPLE"), os.Getenv("ASK_MODEL")
	if key == "" || path == "" || model == "" {
		t.Skip("ANTHROPIC_API_KEY, ASK_SAMPLE and ASK_MODEL are required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var posts []struct {
		Platform, Title, Content string
		OldAsk                   string `json:"old_ask"`
	}
	if err := json.Unmarshal(raw, &posts); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	p := Provider{Name: "anthropic", APIKey: key, Model: model}
	sc := ScoringContext{}
	got := make([]string, len(posts))
	var wg sync.WaitGroup
	for i := range posts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cr, err := ClassifyIntent(ctx, p, posts[i].Title, posts[i].Content, posts[i].Platform, sc)
			if err != nil {
				got[i] = "ERROR " + err.Error()
				return
			}
			got[i] = cr.AuthorRole + " | " + cr.Ask
		}(i)
	}
	wg.Wait()
	for i, post := range posts {
		t.Logf("\nOLD: %s\nNEW: %s", post.OldAsk, got[i])
	}
}
