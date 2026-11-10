package monitor

import (
	"reflect"
	"testing"

	"leadecho/internal/database"
)

func TestWithoutPaidPlatforms(t *testing.T) {
	kws := []database.ListActiveKeywordsRow{
		{ID: "a", Platforms: []string{"reddit", "hackernews", "twitter", "linkedin", "bluesky"}},
		{ID: "b", Platforms: []string{"twitter", "linkedin"}},
		{ID: "c", Platforms: []string{"devto"}},
	}

	got := withoutPaidPlatforms(kws)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("keywords = %+v, want a and c (b has only paid platforms)", got)
	}
	if want := []string{"hackernews", "bluesky"}; !reflect.DeepEqual(got[0].Platforms, want) {
		t.Fatalf("platforms = %v, want %v", got[0].Platforms, want)
	}
	if len(kws[0].Platforms) != 5 {
		t.Fatalf("input was mutated: %v", kws[0].Platforms)
	}
}
