package monitor

import (
	"regexp"
	"strings"
)

// Free checks that keep obvious non-leads away from the paid AI steps.

var wordRe = regexp.MustCompile(`[a-z0-9]+`)

// Words that say nothing about a topic; ignored when comparing a keyword
// with a post.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "how": true, "what": true, "why": true, "who": true,
	"are": true, "you": true, "your": true, "our": true, "from": true, "that": true, "this": true, "can": true,
	"does": true, "not": true, "any": true, "best": true, "good": true, "tool": true, "tools": true,
}

// significantWords are a keyword's topic words: 3+ letters, not stop words.
func significantWords(term string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(term), -1) {
		if len(w) < 3 || stopWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

// keywordOverlap reports whether text is plausibly about the keyword: at
// least two of the keyword's topic words, and at least half of them, appear
// in it, matched
// by word stem so "screen" finds "screening" and "engineer" finds
// "engineers". Search by relevance returns posts that share a single word
// ("assessment" found karate gradings and a driving assessment for the
// keyword "technical assessment tool"); those would each cost an AI call
// just to be rejected.
//
// ponytail: prefix match on words, English only. Use real stemming or
// embeddings if this starts dropping paraphrased buyer posts.
func keywordOverlap(text, term string) bool {
	words := significantWords(term)
	if len(words) < 2 {
		return true // a one-word keyword has nothing to cross-check against
	}
	have := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(text), -1) {
		have[w] = true
	}
	hits := 0
	for _, kw := range words {
		stem := kw
		if len(stem) > 5 {
			stem = stem[:len(stem)-2] // "screening" ~ "screen", "engineers" ~ "engineer"
		}
		for w := range have {
			if strings.HasPrefix(w, stem) {
				hits++
				break
			}
		}
	}
	return hits >= max(2, (len(words)+1)/2) // at least two, and at least half
}

var nonAlnumLower = regexp.MustCompile(`[^a-z0-9]+`)

// ownAccount reports whether an author handle is the workspace's own brand
// account ("UtkrushtAI" for the product "Utkrusht"). The customer's own
// posts are not leads and should not be scored. Short product names are not
// matched: "Go" or "Arc" would hit unrelated handles.
func ownAccount(author, product string) bool {
	a := nonAlnumLower.ReplaceAllString(strings.ToLower(author), "")
	p := nonAlnumLower.ReplaceAllString(strings.ToLower(product), "")
	return len(p) >= 5 && a != "" && strings.Contains(a, p)
}
