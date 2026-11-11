package handler

import (
	"strings"
	"testing"
)

func TestKeywordOrder(t *testing.T) {
	got := keywordOrder([]listenerKeywords{
		{profileID: "content", goal: "content_ideas", terms: []string{"c1", "c2"}},
		{profileID: "buyers", goal: "find_buyers", terms: []string{"b1", "b2", "b3"}},
		{profileID: "switch", goal: "competitor_switchers", terms: []string{"s1"}},
	})
	var terms []string
	for _, k := range got {
		terms = append(terms, k.term)
	}
	if s := strings.Join(terms, ","); s != "b1,s1,c1,b2,c2,b3" {
		t.Fatalf("order = %s", s)
	}
	if got[0].profileID != "buyers" {
		t.Fatalf("first keyword belongs to %q", got[0].profileID)
	}
	if len(keywordOrder(nil)) != 0 {
		t.Fatal("empty input should give no keywords")
	}
}
