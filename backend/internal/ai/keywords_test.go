package ai

import (
	"reflect"
	"strings"
	"testing"
)

func TestCleanSuggestions(t *testing.T) {
	in := []SuggestedKeyword{
		{Term: "  Hinglish speech to text ", Platforms: []string{"Reddit", "twitter", "reddit", "myspace"}},
		{Term: "dictation app for mac", Platforms: []string{"reddit"}},    // exists, other case and spacing below
		{Term: "hinglish  SPEECH to text", Platforms: []string{"reddit"}}, // duplicate of the first
		{Term: "", Platforms: []string{"reddit"}},
		{Term: "wispr flow too expensive", Platforms: []string{"linkedin"}}, // paid platform not allowed
		{Term: "superwhisper alternative", Platforms: []string{"reddit"}},
	}
	got := CleanSuggestions(in, []string{"Dictation  App for Mac"}, []string{"reddit", "hackernews"}, 2)
	want := []SuggestedKeyword{
		{Term: "Hinglish speech to text", Platforms: []string{"reddit"}},
		{Term: "wispr flow too expensive", Platforms: []string{"reddit"}}, // falls back to the first allowed
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestCleanVerdicts(t *testing.T) {
	known := map[string]string{"wispr flow alternative": "Wispr Flow alternative", "lungoor": "Lungoor"}
	got := cleanVerdicts([]KeywordVerdict{
		{Term: "wispr flow ALTERNATIVE", Action: "platforms", Platforms: []string{"reddit", "linkedin"}},
		{Term: "Lungoor", Action: "keep", Platforms: []string{"reddit"}},
		{Term: "Lungoor", Action: "pause"},                // second verdict on the same keyword
		{Term: "made up keyword", Action: "pause"},        // not an existing keyword
		{Term: "Wispr Flow alternative", Action: "boost"}, // unknown action
	}, known, []string{"reddit", "hackernews"})
	want := []KeywordVerdict{
		{Term: "Wispr Flow alternative", Action: "platforms", Platforms: []string{"reddit"}},
		{Term: "Lungoor", Action: "keep"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

// The judge must see the numbers, the posts with why they were scored that
// way, and its own last advice.
func TestReviewInputText(t *testing.T) {
	txt := KeywordReviewInput{
		Brief:     ProductBrief{Product: "Lungoor", Positioning: "Hinglish dictation"},
		Keywords:  []KeywordStat{{Term: "Wispr Flow alternative", Active: true, Platforms: []string{"twitter"}, Posts: 106, Leads: 0, ByPlatform: []string{"twitter 106 posts, 46 judged, 0 leads"}}},
		Posts:     []ReviewPost{{Platform: "reddit", Keyword: "dictation on Mac", Score: 9, Outcome: "LEAD", Text: "Is there a free Wispr Flow alternative?", Reason: "price-sensitive Mac user"}},
		Platforms: []string{"reddit"},
		Previous:  `- add "hinglish dictation": applied; since then 12 posts, 2 leads`,
		Days:      14,
	}.text()
	for _, want := range []string{"Hinglish dictation", `"Wispr Flow alternative" [active] searches twitter: 106 posts, 0 leads`, "LEAD, score 9.0", "scorer: price-sensitive", "since then 12 posts, 2 leads"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
}

// The listener writer must see the product's edge, or a Hinglish dictation
// app gets eleven generic "dictation app for Mac" keywords and none for the
// buyers only it can serve.
func TestListenerPromptCarriesTheEdge(t *testing.T) {
	txt := ListenerPrompt{
		Goal: "find_buyers", Product: "Lungoor Voice",
		Positioning: "turns Hinglish speech into polished English",
		Features:    []string{"60+ languages"},
		Segments:    []string{"Indian SaaS teams: we talk in Hinglish (IN)"},
	}.userText()
	for _, want := range []string{"Hinglish speech", "60+ languages", "Indian SaaS teams", "(IN)"} {
		if !strings.Contains(txt, want) {
			t.Errorf("prompt is missing %q:\n%s", want, txt)
		}
	}
}

func TestClassifyPromptCarriesRules(t *testing.T) {
	p := buildClassifyPrompt(ScoringContext{Product: "- Lungoor", Rules: []string{"  Founders promoting their own app are noise ", ""}})
	if !strings.Contains(p, "RULES LEARNED FOR THIS WORKSPACE") || !strings.Contains(p, "- Founders promoting their own app are noise\n") {
		t.Fatalf("rules missing:\n%s", p)
	}
	if strings.Contains(buildClassifyPrompt(ScoringContext{Product: "- X"}), "RULES LEARNED") {
		t.Error("no rules: no rules section")
	}
}
