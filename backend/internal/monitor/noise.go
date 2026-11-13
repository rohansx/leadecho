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
	// Drop search operators: a "-excluded" word is not a topic word.
	fields := strings.Fields(strings.ToLower(term))
	kept := fields[:0]
	for _, f := range fields {
		if !strings.HasPrefix(f, "-") {
			kept = append(kept, f)
		}
	}
	for _, w := range wordRe.FindAllString(strings.Join(kept, " "), -1) {
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
// in it, matched by word stem so "screen" finds "screening" and "engineer" finds
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
		// One topic word ("Utkrusht", or "best CRM" once "best" is ignored):
		// nothing to cross-check against, so the search result stands.
		return true
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

// onTopic keeps the search results that pass keywordOverlap. Used for
// Reddit's site-wide search on both paths (data provider and public API), so
// the same posts are kept whichever one answers.
func onTopic(posts []Post, term string) []Post {
	out := make([]Post, 0, len(posts))
	for _, p := range posts {
		if keywordOverlap(p.Title+" "+p.Content, term) {
			out = append(out, p)
		}
	}
	return out
}

var nonAlnumLower = regexp.MustCompile(`[^a-z0-9]+`)

// Endings a company's own account commonly adds to its name.
var brandSuffixes = []string{"", "ai", "hq", "app", "io", "inc", "co", "labs", "team", "official", "dev", "tech"}

// ownAccount reports whether an author handle is the workspace's own brand
// account: the product name exactly, or with a common company ending
// ("UtkrushtAI", "utkrusht_hq"). The customer's own posts are not leads.
// A handle that merely contains the name ("notionfan", "slackerdev") is
// somebody else, often a customer or critic worth hearing, and is scored
// as usual.
func ownAccount(author, product string) bool {
	a := nonAlnumLower.ReplaceAllString(strings.ToLower(author), "")
	p := nonAlnumLower.ReplaceAllString(strings.ToLower(product), "")
	if len(p) < 4 || a == "" {
		return false
	}
	for _, suffix := range brandSuffixes {
		if a == p+suffix {
			return true
		}
	}
	// "Utkrusht AI" as the product name and "utkrusht" as the handle.
	for _, suffix := range brandSuffixes[1:] {
		if strings.HasSuffix(p, suffix) && a == strings.TrimSuffix(p, suffix) && len(a) >= 4 {
			return true
		}
	}
	return false
}

// brandNotNamed reports whether a post found by a keyword that names the
// product ("Utkrusht interview") never mentions the product. Searches match
// the keyword's other words, so most results for such a keyword are about
// interviews in general (589 of 593 for one customer); a brand keyword
// exists to find mentions of the brand, and these are not that.
func brandNotNamed(a mentionAlert, product string) bool {
	// The name's first word: a post that says "Utkrusht" names "Utkrusht AI".
	p := ""
	if f := strings.Fields(product); len(f) > 0 {
		p = nonAlnumLower.ReplaceAllString(strings.ToLower(f[0]), "")
	}
	if len(p) < 4 || !strings.Contains(nonAlnumLower.ReplaceAllString(strings.ToLower(a.Keyword), ""), p) {
		return false
	}
	text := strings.ToLower(a.Title + " " + a.Content + " " + a.Author + " " + a.URL)
	return !strings.Contains(nonAlnumLower.ReplaceAllString(text, ""), p)
}
