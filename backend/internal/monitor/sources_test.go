package monitor

import (
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/cadence"
	"leadecho/internal/database"
	"leadecho/internal/mutes"
)

func TestKeywordsDue(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	kws := []database.ListActiveKeywordsRow{
		{ID: "k1", Platforms: []string{"reddit", "twitter"}},
		{ID: "k2", Platforms: []string{"hackernews"}},
		{ID: "k3", Platforms: []string{"bluesky"}},
	}
	cad := map[string]cadence.Map{"k1": {"twitter": "6h"}}
	last := map[string]map[string]time.Time{
		// reddit is a paid (treg) source: default every 4h, so 5h ago is due.
		"k1": {"reddit": now.Add(-5 * time.Hour), "twitter": now.Add(-time.Hour)},
		"k2": {"hackernews": now.Add(-10 * time.Minute)},
	}
	got := keywordsDue(kws, cad, last, now)
	if len(got) != 2 {
		t.Fatalf("got %d keywords, want 2 (k2 not due): %+v", len(got), got)
	}
	if got[0].ID != "k1" || !reflect.DeepEqual(got[0].Platforms, []string{"reddit"}) {
		t.Errorf("k1 = %+v, want only reddit due", got[0])
	}
	// The 4h paid default is what saves the treg balance: 2h ago is not due.
	recent := map[string]map[string]time.Time{"k1": {"reddit": now.Add(-2 * time.Hour), "twitter": now.Add(-time.Hour)}}
	if due := keywordsDue(kws[:1], cad, recent, now); len(due) != 0 {
		t.Errorf("reddit scanned 2h ago must not be due under the 4h default: %+v", due)
	}
	if got[1].ID != "k3" || !reflect.DeepEqual(got[1].Platforms, []string{"bluesky"}) {
		t.Errorf("k3 = %+v, want bluesky (never scanned)", got[1])
	}
	if len(kws[0].Platforms) != 2 {
		t.Error("input keyword slice was mutated")
	}
}

func TestMuteCandidate(t *testing.T) {
	p := database.CreateMentionParams{
		Platform:         "reddit",
		Url:              "https://reddit.com/r/SaaS/x",
		Title:            pgtype.Text{String: "T", Valid: true},
		Content:          "C",
		AuthorUsername:   pgtype.Text{String: "bob", Valid: true},
		PlatformMetadata: []byte(`{"subreddit":"SaaS","score":3}`),
	}
	want := mutes.Candidate{Platform: "reddit", Author: "bob", URL: "https://reddit.com/r/SaaS/x", Title: "T", Content: "C", Subreddit: "SaaS"}
	if got := muteCandidate(p); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// Bad metadata is tolerated.
	p.PlatformMetadata = []byte(`not json`)
	if got := muteCandidate(p); got.Subreddit != "" || got.Author != "bob" {
		t.Errorf("bad metadata: %+v", got)
	}
}
