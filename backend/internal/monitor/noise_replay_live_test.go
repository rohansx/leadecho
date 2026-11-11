//go:build live

// Replays keywordOverlap over posts a real scan collected, to see what the
// rule would have dropped. Run:
//
//	NOISE_SAMPLE=/path/reddit.json go test -tags live -run TestKeywordOverlapReplay -v ./internal/monitor/
//
// reddit.json: [{"term","title","content","score":number,"role","stage"}, ...]
package monitor

import (
	"encoding/json"
	"os"
	"testing"
)

func TestKeywordOverlapReplay(t *testing.T) {
	path := os.Getenv("NOISE_SAMPLE")
	if path == "" {
		t.Skip("NOISE_SAMPLE not set")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var posts []struct {
		Term, Title, Content, Role, Stage string
		Score                             float64
	}
	if err := json.Unmarshal(raw, &posts); err != nil {
		t.Fatal(err)
	}
	var dropped, droppedAI, droppedGood int
	for _, p := range posts {
		if keywordOverlap(p.Title+" "+p.Content, p.Term) {
			continue
		}
		dropped++
		if p.Stage == "stage1_spam_filter" || p.Stage == "stage3_classified" {
			droppedAI++ // an AI call that would have been saved
		}
		if p.Score >= 4.5 || p.Role == "buyer" {
			droppedGood++
			t.Logf("WOULD DROP (score %.1f, %s) [%s]: %.120s", p.Score, p.Role, p.Term, p.Title+" | "+p.Content)
		}
	}
	t.Logf("posts=%d dropped=%d (%.0f%%) ai_calls_saved=%d good_posts_dropped=%d",
		len(posts), dropped, 100*float64(dropped)/float64(max(1, len(posts))), droppedAI, droppedGood)
}
