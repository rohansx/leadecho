package ai

import (
	"strings"
	"testing"
)

// The production case: a hiring manager asked how to run take-homes, and the
// reply drafted for a product that replaces take-homes coached them on it.
func TestDraftReplyPromptHoldsTheProductsPosition(t *testing.T) {
	opts := DraftReplyOptions{
		Title: "Interview Take Homes in 2026", Content: "How long should a take-home be for a Spring/React role?",
		Platform: "reddit", Intent: "complaint", AwarenessLevel: "problem_aware", TemplateStyle: "casual_helpful",
		ProductName:        "Utkrusht",
		ProductDescription: "Candidates solve real problems in a live, running system instead of coding tests or take-homes.",
		Positioning:        "Real work in a live system instead of take-home tests.",
		Voice:              "- Product mentions: Recommend the product clearly by name and lead with its most relevant strength.",
	}
	system, user := draftReplyPrompts(opts)

	for _, want := range []string{
		"What the product does: Candidates solve real problems in a live, running system instead of coding tests or take-homes.",
		"How the product differs: Real work in a live system instead of take-home tests.",
		"Product name: Utkrusht",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("the drafting model is not told %q:\n%s", want, user)
		}
	}
	for _, want := range []string{
		"do not recommend, praise or coach the reader on the approach the product exists to replace",
		"even when you are told not to mention the product",
		"Never name the product, hint at it or pitch anything in the first sentence",
		"No links or URLs of any kind",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("the reply prompt lacks %q", want)
		}
	}
	// The voice profile "takes precedence over the rules above": the rules
	// that may not be overridden have to come after it.
	if v, a := strings.Index(system, "VOICE PROFILE"), strings.Index(system, "ALWAYS (nothing above overrides these)"); v < 0 || a < v {
		t.Errorf("the fixed rules must follow the voice profile (voice at %d, rules at %d)", v, a)
	}
}

func TestDraftReplyPromptWithoutAProductProfile(t *testing.T) {
	system, user := draftReplyPrompts(DraftReplyOptions{Content: "Which queue?", Platform: "reddit", TemplateStyle: "no such style"})
	if strings.Contains(system, "on behalf of the product") || strings.Contains(user, "What the product does") {
		t.Error("no product description: no position to hold")
	}
	if !strings.Contains(system, "Help first") || !strings.Contains(system, `"template_style": "value_first"`) {
		t.Errorf("an unknown style falls back to value_first and keeps the fixed rules:\n%s", system)
	}
}

// The storytelling style used to ask for "I ran into the same issue when..."
// and "what eventually helped (including the product)": an invented account
// from someone posing as a user. No style may ask for that, and every style
// that can name the product is told to say whose product it is.
func TestDraftReplyPromptsNeverFabricateTestimony(t *testing.T) {
	for style, guide := range templatePrompts {
		system, _ := draftReplyPrompts(DraftReplyOptions{Content: "How do you screen senior engineers?", Platform: "reddit", TemplateStyle: style,
			ProductName: "Utkrusht", ProductDescription: "Candidates solve real problems in a live system.",
			Voice: "- Product mentions: Recommend the product clearly by name."})
		for _, want := range []string{
			"Never invent a personal experience, a customer, a colleague, a team or an outcome",
			"Never pose as an independent or unaffiliated user of the product",
			`"I work on <product>"`,
		} {
			if !strings.Contains(system, want) {
				t.Errorf("%s: the reply prompt lacks %q", style, want)
			}
		}
		// The fixed rules come after the style and the voice profile.
		if a, r := strings.Index(system, "ALWAYS (nothing above overrides these)"), strings.Index(system, "Never pose as"); a < 0 || r < a {
			t.Errorf("%s: the affiliation rule must be one of the rules nothing overrides", style)
		}
		low := strings.ToLower(guide)
		for _, not := range []string{"i ran into", "personal story", "relatable experience", "what you tried", "credibility", "i've been", "i used"} {
			if strings.Contains(low, not) {
				t.Errorf("%s asks for an invented first-person account (%q):\n%s", style, not, guide)
			}
		}
	}
	if s := templatePrompts["storytelling"]; !strings.Contains(s, "in general terms") || !strings.Contains(s, "not of anything that happened to you") {
		t.Errorf("storytelling must tell the problem's story, not the writer's:\n%s", s)
	}
}
