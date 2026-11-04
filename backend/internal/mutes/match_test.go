package mutes

import (
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	c := Candidate{
		Platform:  "reddit",
		Author:    "SpamBot42",
		URL:       "https://www.Blog.Example.com/post/1",
		Title:     "Best CRM?",
		Content:   "Check out my Crypto giveaway",
		Subreddit: "SaaS",
	}
	tests := []struct {
		name string
		mute Mute
		c    Candidate
		want bool
	}{
		{"author case-insensitive", Mute{Kind: KindAuthor, Value: "spambot42"}, c, true},
		{"author with @ prefix", Mute{Kind: KindAuthor, Value: "@SpamBot42"}, c, true},
		{"author with u/ prefix", Mute{Kind: KindAuthor, Value: "u/spambot42"}, c, true},
		{"author mismatch", Mute{Kind: KindAuthor, Value: "spambot4"}, c, false},
		{"author empty candidate", Mute{Kind: KindAuthor, Value: "x"}, Candidate{}, false},
		{"domain exact", Mute{Kind: KindDomain, Value: "www.blog.example.com"}, c, true},
		{"domain suffix", Mute{Kind: KindDomain, Value: "example.com"}, c, true},
		{"domain given as url", Mute{Kind: KindDomain, Value: "https://EXAMPLE.com/foo"}, c, true},
		{"domain leading dot", Mute{Kind: KindDomain, Value: ".example.com"}, c, true},
		{"domain not a label suffix", Mute{Kind: KindDomain, Value: "ample.com"}, c, false},
		{"domain other", Mute{Kind: KindDomain, Value: "example.org"}, c, false},
		{"domain bad candidate url", Mute{Kind: KindDomain, Value: "example.com"}, Candidate{URL: "::bad"}, false},
		{"keyword in content", Mute{Kind: KindKeyword, Value: "crypto"}, c, true},
		{"keyword in title", Mute{Kind: KindKeyword, Value: "best crm"}, c, true},
		{"keyword absent", Mute{Kind: KindKeyword, Value: "nft"}, c, false},
		{"subreddit", Mute{Kind: KindSubreddit, Value: "saas"}, c, true},
		{"subreddit r/ prefix", Mute{Kind: KindSubreddit, Value: "/r/SaaS"}, c, true},
		{"subreddit mismatch", Mute{Kind: KindSubreddit, Value: "sales"}, c, false},
		{"subreddit ignored off reddit", Mute{Kind: KindSubreddit, Value: "saas"}, Candidate{Platform: "twitter", Subreddit: "saas"}, false},
		{"unknown kind", Mute{Kind: "foo", Value: "spambot42"}, c, false},
		{"blank value never matches", Mute{Kind: KindKeyword, Value: "  "}, c, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Match([]Mute{tt.mute}, tt.c); got != tt.want {
				t.Errorf("Match(%+v) = %v, want %v", tt.mute, got, tt.want)
			}
		})
	}
	if Match(nil, c) {
		t.Error("no mutes must not match")
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		kind, in, want string
		ok             bool
	}{
		{KindAuthor, " @Bob ", "bob", true},
		{KindDomain, "https://www.Foo.com/x", "www.foo.com", true},
		{KindDomain, ".foo.com", "foo.com", true},
		{KindSubreddit, "r/SaaS", "saas", true},
		{KindKeyword, "  Crypto  ", "crypto", true},
		{KindKeyword, "   ", "", false},
		{"bogus", "x", "", false},
		{KindKeyword, strings.Repeat("a", 300), "", false},
	}
	for _, tt := range tests {
		got, err := Normalize(tt.kind, tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("Normalize(%q,%q) = %q,%v want %q ok=%v", tt.kind, tt.in, got, err, tt.want, tt.ok)
		}
	}
}
