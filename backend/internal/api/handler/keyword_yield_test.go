package handler

import (
	"reflect"
	"testing"

	"leadecho/internal/cadence"
	"leadecho/internal/database"
)

func TestKeywordYields(t *testing.T) {
	rows := []database.KeywordYield{
		{KeywordID: "coding", Term: "coding test", Platform: "hackernews", Posts: 273, Passed: 30, Leads: 0, Asks: 4},
		{KeywordID: "coding", Term: "coding test", Platform: "bluesky", Posts: 242, Passed: 3},
		{KeywordID: "coding", Term: "coding test", Platform: "devto", Posts: 20},
		{KeywordID: "alt", Term: "HackerRank alternative", Platform: "reddit", Posts: 63, Passed: 20, Leads: 6, Asks: 9},
		// Many posts on one platform, but the keyword does find leads elsewhere.
		{KeywordID: "mixed", Term: "hiring developers", Platform: "bluesky", Posts: 180},
		{KeywordID: "mixed", Term: "hiring developers", Platform: "twitter", Posts: 40, Passed: 12, Leads: 2},
	}
	got := keywordYields(rows)
	want := map[string]keywordYield{
		"coding": {Yield: cadence.Yield{Posts: 535, Passed: 33, Asks: 4}, LowYield: true, ThrottledPlatforms: []string{"hackernews", "bluesky"}},
		"alt":    {Yield: cadence.Yield{Posts: 63, Passed: 20, Leads: 6, Asks: 9}},
		"mixed":  {Yield: cadence.Yield{Posts: 220, Passed: 12, Leads: 2}, ThrottledPlatforms: []string{"bluesky"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("yields = %+v\nwant     %+v", got, want)
	}
}
