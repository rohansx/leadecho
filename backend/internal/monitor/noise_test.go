package monitor

import "testing"

// Posts are real results from the first production scan (2026-11-11).
func TestKeywordOverlap(t *testing.T) {
	cases := []struct {
		name, term, text string
		want             bool
	}{
		{"karate grading for an assessment keyword", "technical assessment tool",
			"My son has his karate assessment on Saturday and he is so nervous about the grading", false},
		{"driving assessment", "technical assessment tool",
			"Failed my driving assessment today, the examiner was harsh", false},
		{"real buyer, paraphrased", "how to screen software engineers",
			"How do you screen experienced engineers for judgment, not just implementation?", true},
		{"real buyer, take-home", "take-home assignment hiring",
			"Interview Take Homes in 2026. My company is hiring a Sr. Developer and I plan to give a take home project", true},
		{"hiring manager on cheating", "candidates cheating coding interview",
			"Alumni and hiring manager here. You guys are cheating like crazy in our coding test", true},
		{"competitor alternative", "HackerRank alternative",
			"What are decent HackerRank alternatives on the market at the moment?", true},
		{"one topic word only", "HackerRank alternative",
			"Looking for an alternative to my current gym routine", false},
		{"single-word keyword is not cross-checked", "Utkrusht", "anything at all here", true},
		{"all stop words", "how to", "anything at all here", true},
	}
	for _, c := range cases {
		if got := keywordOverlap(c.text, c.term); got != c.want {
			t.Errorf("%s: keywordOverlap = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestOwnAccount(t *testing.T) {
	own := [][2]string{{"UtkrushtAI", "Utkrusht"}, {"utkrusht_ai", "Utkrusht AI"}, {"Utkrusht", "utkrusht"}}
	for _, p := range own {
		if !ownAccount(p[0], p[1]) {
			t.Errorf("%q should be recognised as %q's own account", p[0], p[1])
		}
	}
	notOwn := [][2]string{{"Frazzlebilt", "Utkrusht"}, {"", "Utkrusht"}, {"golang_fan", "Go"}, {"archer", "Arc"}, {"someone", ""}}
	for _, p := range notOwn {
		if ownAccount(p[0], p[1]) {
			t.Errorf("%q must not match product %q", p[0], p[1])
		}
	}
}
