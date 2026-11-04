package reply

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"leadecho/internal/database"
)

// Voice is a response profile reduced to what the drafter needs.
type Voice struct {
	Goal                string            `json:"goal"`
	SpecialInstructions string            `json:"special_instructions"`
	Tone                int               `json:"tone"`          // 0 casual .. 100 formal
	Length              string            `json:"length"`        // short | medium | long
	Assertiveness       int               `json:"assertiveness"` // 0 tentative .. 100 direct
	BrandPlug           string            `json:"brand_plug"`    // none | low | medium | high
	CTAGoal             string            `json:"cta_goal"`
	DestinationURL      string            `json:"destination_url"`
	Examples            map[string]string `json:"examples"`
}

const maxVoiceText = 2000

var goalText = map[string]string{
	"drive_conversion":   "move the reader toward trying the product, without being pushy",
	"build_relationship": "build goodwill and credibility; be generous and human",
	"ask_for_info":       "ask one good clarifying question to learn more about their situation",
	"handle_feedback":    "acknowledge the feedback honestly and show it is heard",
	"support":            "solve the reader's problem step by step",
	"custom":             "",
}

var lengthText = map[string]string{
	"short":  "1-2 sentences",
	"medium": "2-4 sentences",
	"long":   "up to two short paragraphs",
}

var plugText = map[string]string{
	"none":   "Do NOT mention the product at all.",
	"low":    "Mention the product at most once, briefly, as one option, only after giving real help.",
	"medium": "Mention the product once by name with a concrete reason it fits this case.",
	"high":   "Recommend the product clearly by name and lead with its most relevant strength.",
}

// ExamplePlatforms are the per-platform example keys a profile may hold.
var ExamplePlatforms = []string{"reddit", "x", "linkedin", "bluesky", "hackernews"}

// DefaultVoice is the voice used when a workspace has no response profile.
func DefaultVoice() Voice {
	return Voice{Goal: "drive_conversion", Tone: 40, Length: "medium", Assertiveness: 40, BrandPlug: "low", Examples: map[string]string{}}
}

// formalBrandRE flags brands whose buyers expect a more formal register.
var formalBrandRE = regexp.MustCompile(`(?i)\b(enterprise|compliance|soc ?2|hipaa|security|legal|finance|bank|healthcare|government)\b`)

// DerivedVoice is DefaultVoice with the tone nudged by the brand description.
// ponytail: keyword heuristic; swap for a tone field on the brand profile once it exists.
func DerivedVoice(brandDescription string) Voice {
	v := DefaultVoice()
	if formalBrandRE.MatchString(brandDescription) {
		v.Tone = 65
	}
	return v
}

// VoiceFromProfile converts a stored profile; bad examples JSON yields no examples.
func VoiceFromProfile(p database.ResponseProfile) Voice {
	ex := map[string]string{}
	if len(p.Examples) > 0 {
		if err := json.Unmarshal(p.Examples, &ex); err != nil || ex == nil {
			ex = map[string]string{}
		}
	}
	return Voice{
		Goal: p.Goal, SpecialInstructions: p.SpecialInstructions, Tone: int(p.Tone), Length: p.Length,
		Assertiveness: int(p.Assertiveness), BrandPlug: p.BrandPlug, CTAGoal: p.CtaGoal,
		DestinationURL: p.DestinationUrl, Examples: ex,
	}
}

// ValidateVoice checks user input at the API boundary.
func ValidateVoice(v Voice) error {
	if _, ok := goalText[v.Goal]; !ok {
		return errors.New("goal must be one of drive_conversion, build_relationship, ask_for_info, handle_feedback, support, custom")
	}
	if v.Tone < 0 || v.Tone > 100 {
		return errors.New("tone must be between 0 and 100")
	}
	if v.Assertiveness < 0 || v.Assertiveness > 100 {
		return errors.New("assertiveness must be between 0 and 100")
	}
	if _, ok := lengthText[v.Length]; !ok {
		return errors.New("length must be short, medium or long")
	}
	if _, ok := plugText[v.BrandPlug]; !ok {
		return errors.New("brand_plug must be none, low, medium or high")
	}
	if len(v.SpecialInstructions) > maxVoiceText || len(v.CTAGoal) > maxVoiceText {
		return fmt.Errorf("special_instructions and cta_goal must be at most %d characters", maxVoiceText)
	}
	if v.DestinationURL != "" {
		u, err := url.Parse(v.DestinationURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("destination_url must be an http(s) URL")
		}
	}
	for k, ex := range v.Examples {
		if !isExamplePlatform(k) {
			return fmt.Errorf("examples: unknown platform %q (use %s)", k, strings.Join(ExamplePlatforms, ", "))
		}
		if len(ex) > maxVoiceText {
			return fmt.Errorf("examples: %s example must be at most %d characters", k, maxVoiceText)
		}
	}
	return nil
}

func isExamplePlatform(k string) bool {
	for _, p := range ExamplePlatforms {
		if p == k {
			return true
		}
	}
	return false
}

// ExampleForPlatform returns the style sample for a mention's platform.
func ExampleForPlatform(examples map[string]string, platform string) string {
	if platform == "twitter" {
		platform = "x"
	}
	return strings.TrimSpace(examples[platform])
}

// VoiceInstructions turns presets and sliders into prompt instructions.
// Unknown values fall back to the default voice's.
func VoiceInstructions(v Voice) string {
	def := DefaultVoice()
	goal, ok := goalText[v.Goal]
	if !ok {
		goal = goalText[def.Goal]
	}
	length, ok := lengthText[v.Length]
	if !ok {
		length = lengthText[def.Length]
	}
	plug, ok := plugText[v.BrandPlug]
	if !ok {
		plug = plugText[def.BrandPlug]
	}

	var b strings.Builder
	if goal != "" {
		fmt.Fprintf(&b, "- Goal: %s.\n", goal)
	}
	fmt.Fprintf(&b, "- Tone: %s.\n", toneText(v.Tone))
	fmt.Fprintf(&b, "- Length: %s.\n", length)
	fmt.Fprintf(&b, "- Assertiveness: %s\n", assertText(v.Assertiveness))
	fmt.Fprintf(&b, "- Product mentions: %s\n", plug)
	if cta := strings.TrimSpace(v.CTAGoal); cta != "" && v.BrandPlug != "none" {
		fmt.Fprintf(&b, "- End with a soft call to action: %s.\n", cta)
	}
	if si := strings.TrimSpace(v.SpecialInstructions); si != "" {
		fmt.Fprintf(&b, "- Special instructions: %s\n", si)
	}
	return strings.TrimRight(b.String(), "\n")
}

func toneText(t int) string {
	switch {
	case t < 34:
		return "casual, like a friend in the thread; contractions and plain words"
	case t < 67:
		return "conversational and clear"
	default:
		return "formal and professional; no slang"
	}
}

func assertText(a int) string {
	switch {
	case a < 34:
		return "Suggest gently and hedge; offer ideas, not verdicts."
	case a < 67:
		return "Be confident but open to other approaches."
	default:
		return "Be direct; give a clear recommendation."
	}
}

// LinkedReply makes sure the tracked short link is in the reply: it replaces a
// URL to the destination host or follows a product mention (WithTrackingLink),
// and otherwise appends it. Used when brand_plug != none.
func LinkedReply(text, destinationURL, short string) string {
	if strings.Contains(text, short) {
		return text
	}
	if linked, ok := WithTrackingLink(text, destinationURL, short); ok {
		return linked
	}
	return strings.TrimRight(text, "\n") + "\n\n" + short
}

// WithTrackingLink puts the tracked short link into a reply only where the
// product belongs: it replaces a URL pointing at the destination's host, or
// appends the link when the reply names the product. A reply that never
// mentions the product is left alone — a bare link there reads as spam.
func WithTrackingLink(text, destinationURL, short string) (string, bool) {
	u, err := url.Parse(destinationURL)
	if err != nil || u.Hostname() == "" {
		return text, false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")

	hostRE := regexp.MustCompile(`(?i)\b(?:https?://)?(?:www\.)?` + regexp.QuoteMeta(host) + `\b(?:/[^\s)\]>]*[^\s)\]>.,!?;:])?/?`)
	if hostRE.MatchString(text) {
		return hostRE.ReplaceAllLiteralString(text, short), true
	}

	// ponytail: brand = first host label ("cloakpipe" for cloakpipe.co). Misses
	// products whose name differs from their domain; add a product-name setting then.
	brand := strings.SplitN(host, ".", 2)[0]
	if len(brand) >= 3 && strings.Contains(strings.ToLower(text), brand) {
		return strings.TrimRight(text, "\n") + "\n\n" + short, true
	}
	return text, false
}
