package monitor

import (
	"testing"
	"time"

	"leadecho/internal/database"
)

func TestThreadKey(t *testing.T) {
	const r = "reddit:1abc2d"
	cases := map[string]string{
		"https://www.reddit.com/r/SaaS/comments/1abc2d/how_do_i_find_customers/":        r,
		"https://old.reddit.com/r/SaaS/comments/1abc2d/how_do_i_find_customers":         r,
		"https://np.reddit.com/r/SaaS/comments/1abc2d/":                                 r,
		"https://reddit.com/r/SaaS/comments/1abc2d/other_slug/kx9zz1/?utm_source=share": r,
		"https://m.reddit.com/r/saas/comments/1ABC2D/x?context=3#top":                   r,
		"https://redd.it/1abc2d":                                      r,
		"https://www.reddit.com/r/SaaS/":                              "reddit.com/r/saas",
		"https://news.ycombinator.com/item?id=4242&p=2":               "hn:4242",
		"https://news.ycombinator.com/item?id=4242":                   "hn:4242",
		"https://news.ycombinator.com/news":                           "",
		"https://dev.to/ann/finding-first-customers-3kd2/":            "dev.to/ann/finding-first-customers-3kd2",
		"https://www.dev.to/ann/finding-first-customers-3kd2?ref=x#c": "dev.to/ann/finding-first-customers-3kd2",
		"https://example.com/":                                        "",
		"not a url":                                                   "",
		"":                                                            "",
	}
	for in, want := range cases {
		if got := threadKey(in); got != want {
			t.Errorf("threadKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSearchRankQueries(t *testing.T) {
	threads := []database.SearchRankThread{{Keyword: "crm"}, {Keyword: "b"}, {Keyword: "crm"}, {Keyword: "a"}, {Keyword: " "}}
	got := searchRankQueries(threads, 2)
	if len(got) != 2 || got[0] != "crm" || got[1] != "a" {
		t.Fatalf("queries = %v", got)
	}
}

func TestMatchSearchRanks(t *testing.T) {
	now := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	threads := []database.SearchRankThread{
		{MentionID: "m1", URL: "https://www.reddit.com/r/SaaS/comments/abc/slug/"},
		{MentionID: "m2", URL: "https://news.ycombinator.com/item?id=7", RankQuery: "crm"},     // dropped out of "crm"
		{MentionID: "m3", URL: "https://dev.to/ann/post", RankQuery: "not searched this week"}, // kept as is
		{MentionID: "m4", URL: "https://dev.to/bob/post"},                                      // never ranked
	}
	results := map[string][]string{
		"crm":       {"https://example.com/a", "https://old.reddit.com/r/SaaS/comments/abc/other/", "https://x.com/y"},
		"best tool": {"https://reddit.com/r/SaaS/comments/abc/"},
	}
	got := matchSearchRanks(threads, results, now)
	if r := got["m1"]; r == nil || r.Query != "best tool" || r.Position != 1 || !r.CheckedAt.Equal(now) {
		t.Fatalf("m1 = %+v", r)
	}
	if r, ok := got["m2"]; !ok || r != nil {
		t.Fatalf("m2 must be cleared, got %+v %v", r, ok)
	}
	if _, ok := got["m3"]; ok {
		t.Fatal("m3: its query was not searched, leave it")
	}
	if _, ok := got["m4"]; ok {
		t.Fatal("m4: nothing to change")
	}
}

func TestMatchSearchRanksEmptyResultsClearNothing(t *testing.T) {
	threads := []database.SearchRankThread{{MentionID: "m1", URL: "https://news.ycombinator.com/item?id=7", RankQuery: "crm"}}
	for _, empty := range [][]string{nil, {}} {
		if got := matchSearchRanks(threads, map[string][]string{"crm": empty}, time.Now()); len(got) != 0 {
			t.Fatalf("an empty result list must not clear ranks, got %+v", got)
		}
	}
}
