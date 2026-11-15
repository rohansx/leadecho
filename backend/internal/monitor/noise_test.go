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
	own := [][2]string{{"UtkrushtAI", "Utkrusht"}, {"utkrusht_ai", "Utkrusht AI"}, {"Utkrusht", "utkrusht"}, {"utkrusht", "Utkrusht AI"}, {"acme_hq", "Acme"}}
	for _, p := range own {
		if !ownAccount(p[0], p[1]) {
			t.Errorf("%q should be recognised as %q's own account", p[0], p[1])
		}
	}
	// A handle that only contains the name is someone else: a fan, a critic, a customer.
	notOwn := [][2]string{
		{"Frazzlebilt", "Utkrusht"}, {"", "Utkrusht"}, {"golang_fan", "Go"}, {"archer", "Arc"}, {"someone", ""},
		{"notionfan", "Notion"}, {"notionhater", "Notion"}, {"slackerdev", "Slack"}, {"ex_utkrusht_user", "Utkrusht"},
	}
	for _, p := range notOwn {
		if ownAccount(p[0], p[1]) {
			t.Errorf("%q must not match product %q", p[0], p[1])
		}
	}
}

func TestSignificantWordsIgnoresSearchOperators(t *testing.T) {
	got := significantWords(`"coding test" cheating -leetcode`)
	want := []string{"coding", "test", "cheating"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestOnTopic(t *testing.T) {
	posts := []Post{
		{Title: "How do you screen experienced engineers for judgment?", Content: "…"},
		{Title: "Screen protector recommendations", Content: "for my new phone"},
	}
	got := onTopic(posts, "how to screen software engineers")
	if len(got) != 1 || got[0].Title != posts[0].Title {
		t.Fatalf("got %+v", got)
	}
}

// A keyword that names the product is for finding mentions of it: a result
// that only shares the keyword's other word is dropped before any paid step.
func TestBrandNotNamed(t *testing.T) {
	post := func(kw, text string) mentionAlert { return mentionAlert{Keyword: kw, Content: text} }
	utkrusht := newBrand("Utkrusht", "")
	if !brandNotNamed(post("Utkrusht interview", "My Amazon interview went badly"), utkrusht) {
		t.Error("a brand keyword's result that never names the brand is noise")
	}
	for _, keep := range []mentionAlert{
		post("Utkrusht interview", "Took the utkrusht.ai assessment yesterday"),
		post("coding test", "My Amazon interview went badly"),               // not a brand keyword
		{Keyword: "Utkrusht", Content: "great post", Author: "utkrusht_hq"}, // named by the author
	} {
		if brandNotNamed(keep, utkrusht) {
			t.Errorf("kept post dropped: %+v", keep)
		}
	}
	if brandNotNamed(post("utkrusht ai interview", "I tried Utkrusht last week"), newBrand("Utkrusht AI", "")) {
		t.Error("the name's first word is enough to name the product")
	}
	if brandNotNamed(post("Arc review", "nothing here"), newBrand("Arc", "")) {
		t.Error("a name too short to match safely is never used")
	}
}

// The customer's domain names the brand too, both in a keyword and in a post,
// including when the product name is too short to match on its own.
func TestBrandDomain(t *testing.T) {
	arc := newBrand("Arc", "https://www.arc.net/pricing")
	tests := []struct {
		name          string
		keyword, text string
		miss          bool
	}{
		{"domain keyword, post about something else", "arc.net review", "Reviewing my welding arc setup", true},
		{"domain keyword, post links the site", "arc.net review", "Switched to https://arc.net last month", false},
		{"short name alone is not a brand keyword", "Arc review", "nothing here", false},
		{"not a brand keyword", "browser for teams", "nothing here", false},
	}
	for _, tt := range tests {
		if got := arc.missing(tt.keyword, tt.text); got != tt.miss {
			t.Errorf("%s: missing = %v, want %v", tt.name, got, tt.miss)
		}
	}
	if (brand{}).missing("Utkrusht interview", "anything") {
		t.Error("no brand profile: nothing is dropped")
	}
}

// Sources that support exact phrases get the brand quoted, so the search
// cannot answer with posts that only share the keyword's other words.
func TestQuoteBrand(t *testing.T) {
	b := newBrand("Utkrusht AI", "https://utkrusht.ai")
	for term, want := range map[string]string{
		"Utkrusht interview":    `"Utkrusht" interview`,
		"utkrusht.ai":           `"utkrusht.ai"`,
		"coding test":           "coding test",
		`"Utkrusht" assessment`: `"Utkrusht" assessment`, // already quoted by the customer
	} {
		if got := b.quote(term); got != want {
			t.Errorf("quote(%q) = %q, want %q", term, got, want)
		}
	}
	if got := (brand{}).quote("Utkrusht interview"); got != "Utkrusht interview" {
		t.Errorf("no brand: term must be unchanged, got %q", got)
	}
}
