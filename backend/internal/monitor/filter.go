package monitor

import (
	"strings"
	"unicode"

	"leadecho/internal/database"
)

// filterContent returns true if the post content passes relevance checks
// for the given keyword. It applies negative terms, match type, and
// minimum content length filters.
func filterContent(content string, kw database.ListActiveKeywordsRow) bool {
	if len(content) < 20 {
		return false
	}

	lower := strings.ToLower(content)
	term := strings.ToLower(kw.Term)

	// Reject if any negative term appears
	for _, neg := range kw.NegativeTerms {
		if neg == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(neg)) {
			return false
		}
	}

	// Match type filtering
	switch kw.MatchType {
	case "exact":
		// Term must appear as a standalone word (bounded by non-alphanumeric chars)
		if !containsWord(lower, term) {
			return false
		}
	case "phrase":
		// Term must appear as an exact substring
		if !strings.Contains(lower, term) {
			return false
		}
	default:
		// "broad" / "contains" — API search already matched, pass through
	}

	return true
}

// containsWord checks if term appears as a whole word in text.
func containsWord(text, term string) bool {
	idx := 0
	for {
		pos := strings.Index(text[idx:], term)
		if pos < 0 {
			return false
		}
		pos += idx
		start := pos
		end := pos + len(term)

		startOK := start == 0 || !isWordChar(rune(text[start-1]))
		endOK := end == len(text) || !isWordChar(rune(text[end]))

		if startOK && endOK {
			return true
		}
		idx = pos + 1
	}
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
