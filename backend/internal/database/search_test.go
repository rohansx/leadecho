package database

import "testing"

func TestLikePatternMatchesLiterally(t *testing.T) {
	for in, want := range map[string]string{
		"jane":       "%jane%",
		" 50% off ":  `%50\% off%`,
		"snake_case": `%snake\_case%`,
		`a\b`:        `%a\\b%`,
	} {
		if got := likePattern(in); got != want {
			t.Errorf("likePattern(%q) = %q, want %q", in, got, want)
		}
	}
}
