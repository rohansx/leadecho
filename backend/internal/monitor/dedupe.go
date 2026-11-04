package monitor

import (
	"hash/fnv"
	"math/bits"
	"net/url"
	"strings"
	"unicode"
)

// scoring: pre-insert dedupe (canonical URL + near-duplicate text) and bot filter.

const (
	dedupeLookbackDays = 7
	// nearDupBits: max simhash Hamming distance treated as the same post. One
	// changed word in a ~20-word post measures ~6; unrelated posts ~26-32.
	nearDupBits = 6
	// minFuzzyTokens: below this many words simhash is too noisy, so only an
	// exact normalized-text match counts as a duplicate.
	minFuzzyTokens = 8
)

var (
	trackingParams = map[string]bool{
		"fbclid": true, "gclid": true, "dclid": true, "msclkid": true, "mc_cid": true,
		"mc_eid": true, "ref": true, "ref_src": true, "ref_url": true, "igshid": true,
		"si": true, "share_id": true, "rdt": true, "_ga": true, "sh": true,
	}
	// xParams are share-tracking params on X/Twitter links.
	xParams      = map[string]bool{"s": true, "t": true}
	redditPrefix = []string{"www.", "old.", "new.", "np.", "m.", "i."}

	botAuthors = map[string]bool{
		"automoderator": true, "[deleted]": true, "[removed]": true, "deleted": true,
	}
	// botSuffixExceptions are real names that end in "bot".
	// ponytail: tiny list; extend when a real user gets filtered.
	botSuffixExceptions = map[string]bool{"talbot": true, "abbot": true, "cabot": true, "thibot": true}
	botSignatures       = []string{
		"i am a bot", "i'm a bot", "this action was performed automatically", "beep boop",
	}
)

// CanonicalURL normalizes a URL so the same post found through different
// links compares equal: https, lower-case host, no www, reddit old/new/np/m
// subdomains folded, twitter.com→x.com, tracking params and fragments
// dropped, remaining params sorted, no trailing slash.
func CanonicalURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	if strings.HasSuffix(host, "reddit.com") {
		for _, p := range redditPrefix {
			host = strings.TrimPrefix(host, p)
		}
	}
	if host == "twitter.com" || host == "mobile.twitter.com" || host == "m.twitter.com" {
		host = "x.com"
	}

	q := u.Query()
	for k := range q {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "utm_") || trackingParams[lk] || (host == "x.com" && xParams[lk]) {
			q.Del(k)
		}
	}

	out := url.URL{Scheme: "https", Host: host, Path: strings.TrimRight(u.Path, "/"), RawQuery: q.Encode()}
	return out.String()
}

// NormalizeText lower-cases, drops URLs and punctuation and collapses spaces.
func NormalizeText(s string) string {
	words := strings.Fields(strings.ToLower(s))
	kept := make([]string, 0, len(words))
	for _, w := range words {
		if strings.HasPrefix(w, "http://") || strings.HasPrefix(w, "https://") || strings.HasPrefix(w, "www.") {
			continue
		}
		kept = append(kept, w)
	}
	clean := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, strings.Join(kept, " "))
	return strings.Join(strings.Fields(clean), " ")
}

// SimHash is a 64-bit Charikar simhash over the words of normalized text.
func SimHash(normalized string) uint64 {
	var v [64]int
	for _, w := range strings.Fields(normalized) {
		h := fnv.New64a()
		_, _ = h.Write([]byte(w))
		x := h.Sum64()
		for i := 0; i < 64; i++ {
			if x&(1<<uint(i)) != 0 {
				v[i]++
			} else {
				v[i]--
			}
		}
	}
	var out uint64
	for i := 0; i < 64; i++ {
		if v[i] > 0 {
			out |= 1 << uint(i)
		}
	}
	return out
}

// HammingDistance counts differing bits.
func HammingDistance(a, b uint64) int { return bits.OnesCount64(a ^ b) }

// nearDupMaxDistance is the simhash distance to use for this text: -1 = no
// text check (empty), 0 = exact only (short), nearDupBits otherwise.
func nearDupMaxDistance(normalized string) int {
	n := len(strings.Fields(normalized))
	switch {
	case n == 0:
		return -1
	case n < minFuzzyTokens:
		return 0
	}
	return nearDupBits
}

// IsBotMention reports authors and bodies that are automated: AutoModerator,
// deleted accounts, *bot / [bot] names and standard mod-bot footers.
func IsBotMention(author, content string) bool {
	a := strings.ToLower(strings.TrimSpace(author))
	if botAuthors[a] || strings.Contains(a, "[bot]") {
		return true
	}
	if strings.HasSuffix(a, "bot") && len(a) > 3 && !botSuffixExceptions[a] {
		return true
	}
	c := strings.ReplaceAll(strings.ToLower(content), "^", "")
	for _, sig := range botSignatures {
		if strings.Contains(c, sig) {
			return true
		}
	}
	return false
}
