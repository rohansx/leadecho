package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Keyword suggestion and review: the two AI jobs that keep a workspace's
// keywords good. SuggestKeywords fills a keyword list out ("generate 20
// more"); ReviewKeywords is the judge that reads what the keywords actually
// found, as a founder would, and says what to add, pause or move.

// KeywordPlatforms are the sources a suggestion may name (a subset of the
// platform_type enum; the paid ones are only offered when the workspace has
// paid data).
var KeywordPlatforms = []string{"reddit", "hackernews", "twitter", "linkedin", "devto", "bluesky", "exa", "lobsters", "indiehackers"}

// ProductBrief is what the keyword jobs know about the product.
type ProductBrief struct {
	Product     string
	Overview    string
	Positioning string
	Audience    string
	Features    []string
	Segments    []string // "name: pain (country)"
	Competitors []string
	PainPoints  []string
}

func (b ProductBrief) text() string {
	var s strings.Builder
	fmt.Fprintf(&s, "Product: %s\n", b.Product)
	line := func(label, v string) {
		if v = strings.TrimSpace(v); v != "" {
			fmt.Fprintf(&s, "%s: %s\n", label, v)
		}
	}
	line("What it does", b.Overview)
	line("Positioning (the product's edge)", b.Positioning)
	line("Audience", b.Audience)
	line("Features", strings.Join(b.Features, "; "))
	line("Buyer segments", strings.Join(b.Segments, "; "))
	line("Competitors", strings.Join(b.Competitors, ", "))
	line("Pain points", strings.Join(b.PainPoints, "; "))
	return s.String()
}

// SuggestedKeyword is one keyword the AI proposes adding.
type SuggestedKeyword struct {
	Term      string   `json:"term"`
	Platforms []string `json:"platforms"`
	Angle     string   `json:"angle,omitempty"` // buyer | competitor | edge | problem
	Why       string   `json:"why"`
}

const keywordRules = `How good keywords look (each is searched as a phrase on social sites):
- 2-5 words a BUYER types in a post when they need this: "recommend a <category>", "<competitor> alternative", "<competitor> too expensive", "switching from <competitor>", "<problem in their words>". Not marketing copy, not a bare topic ("meeting notes" finds people chatting about the subject, not buyers)
- Cover the product's edge: when the positioning, a feature or a segment names something specific (a language such as Hinglish, a region, an industry, an integration, privacy, a price), write phrases a buyer with THAT need types. Those buyers have the fewest alternatives
- Avoid phrases a competitor's founder uses to promote their own app ("open-source X alternative", "I built a X"): in crowded markets these mostly find launch posts
- Avoid words with another common meaning (a brand that is also a food or a word)
- Platforms, chosen by where these buyers post: reddit (most buyers ask here), hackernews (technical and startup buyers), twitter (complaints and "anyone know a tool" asks; promotion-heavy for consumer and indie tools), linkedin (business buyers; priciest, rarely yields leads, so only for B2B buyers who post there), devto and lobsters (developers picking a developer tool only), bluesky, indiehackers (founders), exa (web-wide search)`

const suggestSystemPrompt = `You write social-listening keywords that find people who are likely to buy a product.

` + keywordRules + `

Return ONLY valid JSON (no markdown): {"keywords": [{"term": "...", "platforms": ["reddit", ...], "angle": "buyer|competitor|edge|problem", "why": "one short sentence"}]}
Never repeat or lightly reword an existing keyword. Use only the allowed platforms.`

// KeywordSuggestRequest asks for Count new keywords.
type KeywordSuggestRequest struct {
	Brief     ProductBrief
	Existing  []string
	Platforms []string // allowed
	Count     int
}

// SuggestKeywords proposes new keywords for a product.
func SuggestKeywords(ctx context.Context, p Provider, req KeywordSuggestRequest) ([]SuggestedKeyword, error) {
	var user strings.Builder
	user.WriteString(req.Brief.text())
	fmt.Fprintf(&user, "\nExisting keywords (do not repeat): %s\n", orNone(req.Existing))
	fmt.Fprintf(&user, "Allowed platforms: %s\n", strings.Join(req.Platforms, ", "))
	fmt.Fprintf(&user, "Write %d new keywords, best first, spread across the angles (at least a quarter on the product's edge when it has one).\n", req.Count)
	result, err := callChat(ctx, p, []chatMessage{
		{Role: "system", Content: suggestSystemPrompt},
		{Role: "user", Content: user.String()},
	}, 0.4)
	if err != nil {
		return nil, err
	}
	var out struct {
		Keywords []SuggestedKeyword `json:"keywords"`
	}
	if err := json.Unmarshal([]byte(stripCodeFences(result)), &out); err != nil {
		return nil, fmt.Errorf("parse keyword suggestions: %w", err)
	}
	return CleanSuggestions(out.Keywords, req.Existing, req.Platforms, req.Count), nil
}

// CleanSuggestions drops empty, duplicate and existing terms, keeps only the
// allowed platforms (a suggestion left with none gets the first allowed one),
// and caps the list at max.
func CleanSuggestions(in []SuggestedKeyword, existing, allowed []string, max int) []SuggestedKeyword {
	seen := map[string]bool{}
	for _, e := range existing {
		seen[normTerm(e)] = true
	}
	ok := map[string]bool{}
	for _, p := range allowed {
		ok[p] = true
	}
	out := []SuggestedKeyword{}
	for _, s := range in {
		s.Term = strings.TrimSpace(s.Term)
		key := normTerm(s.Term)
		if key == "" || seen[key] || len([]rune(s.Term)) > 100 {
			continue
		}
		seen[key] = true
		var plats []string
		dup := map[string]bool{}
		for _, p := range s.Platforms {
			p = strings.ToLower(strings.TrimSpace(p))
			if ok[p] && !dup[p] {
				dup[p] = true
				plats = append(plats, p)
			}
		}
		if len(plats) == 0 && len(allowed) > 0 {
			plats = []string{allowed[0]}
		}
		s.Platforms = plats
		out = append(out, s)
		if max > 0 && len(out) == max {
			break
		}
	}
	return out
}

func normTerm(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func orNone(list []string) string {
	if len(list) == 0 {
		return "(none)"
	}
	return strings.Join(list, ", ")
}

// KeywordStat is what one keyword found over the review window.
type KeywordStat struct {
	Term      string
	Active    bool
	Platforms []string
	// Per platform: "reddit 79 posts, 34 judged, 6 leads".
	ByPlatform []string
	Posts      int
	Leads      int
}

// ReviewPost is one post shown to the judge.
type ReviewPost struct {
	Platform string
	Keyword  string
	Score    float64 // 0 = not scored
	Outcome  string  // "lead", "scored", "dropped by noise filter", ...
	Text     string
	Reason   string // why the scorer decided what it did
}

// KeywordReviewInput is everything the judge reads.
type KeywordReviewInput struct {
	Brief     ProductBrief
	Keywords  []KeywordStat
	Posts     []ReviewPost
	Platforms []string // allowed for new keywords
	// Previous is the last review's suggestions and what became of them, so
	// the judge can see whether its own advice worked.
	Previous string
	Days     int
}

// KeywordVerdict is the judge's call on one existing keyword.
type KeywordVerdict struct {
	Term      string   `json:"term"`
	Action    string   `json:"action"` // keep | pause | platforms
	Platforms []string `json:"platforms,omitempty"`
	Why       string   `json:"why"`
}

// ScoringNote flags a post the scorer judged wrongly.
type ScoringNote struct {
	Post string `json:"post"` // a short quote that identifies it
	Note string `json:"note"`
}

// KeywordReview is the judge's report.
type KeywordReview struct {
	Summary     string             `json:"summary"`
	LeadQuality string             `json:"lead_quality"`
	Verdicts    []KeywordVerdict   `json:"verdicts"`
	Add         []SuggestedKeyword `json:"add"`
	Scoring     []ScoringNote      `json:"scoring"`
}

const reviewSystemPrompt = `You are the judge in a social-listening product. You read what a workspace's keywords actually found, as the product's founder would, and decide what to change so next week brings more real buyers and less noise.

For each post ask: would the founder reply to this person, and might this person pay? A real lead is a person with the problem, shopping or unhappy with what they use, who the product fits (platform, region, budget). Vendor launches, happy users of a competitor, news, listicles and jokes are noise even when on-topic.

` + keywordRules + `

Judge with the numbers: a keyword with many posts and no leads is costing money; a platform that never yields a lead for this product should be dropped from its keywords. Name what is working too, so it is kept.

Return ONLY valid JSON (no markdown):
{"summary": "3-5 sentences: what is working, what is not, the one change that matters most",
 "lead_quality": "one sentence on the leads found: are they people the founder would pay to reach?",
 "verdicts": [{"term": "<existing keyword exactly>", "action": "keep|pause|platforms", "platforms": [...only for action platforms: the full new list], "why": "one sentence"}],
 "add": [{"term": "...", "platforms": [...], "angle": "buyer|competitor|edge|problem", "why": "one sentence citing what you saw"}],
 "scoring": [{"post": "a short quote from a post the scorer got wrong", "note": "what it should have been and why"}]}
Give a verdict for every existing keyword. Suggest 5-15 new keywords. "scoring" may be empty.`

// ReviewKeywords runs the judge.
func ReviewKeywords(ctx context.Context, p Provider, in KeywordReviewInput) (*KeywordReview, error) {
	result, err := callChat(ctx, p, []chatMessage{
		{Role: "system", Content: reviewSystemPrompt},
		{Role: "user", Content: in.text()},
	}, 0.2)
	if err != nil {
		return nil, err
	}
	var r KeywordReview
	if err := json.Unmarshal([]byte(stripCodeFences(result)), &r); err != nil {
		return nil, fmt.Errorf("parse keyword review: %w", err)
	}
	existing := make([]string, 0, len(in.Keywords))
	known := map[string]string{}
	for _, k := range in.Keywords {
		existing = append(existing, k.Term)
		known[normTerm(k.Term)] = k.Term
	}
	r.Add = CleanSuggestions(r.Add, existing, in.Platforms, 15)
	r.Verdicts = cleanVerdicts(r.Verdicts, known, in.Platforms)
	return &r, nil
}

// cleanVerdicts keeps verdicts on keywords that exist, with a known action
// and, for a platform change, a non-empty list of allowed platforms.
func cleanVerdicts(in []KeywordVerdict, known map[string]string, allowed []string) []KeywordVerdict {
	out := []KeywordVerdict{}
	seen := map[string]bool{}
	for _, v := range in {
		term, ok := known[normTerm(v.Term)]
		if !ok || seen[term] {
			continue
		}
		v.Term = term
		switch v.Action {
		case "keep", "pause":
			v.Platforms = nil
		case "platforms":
			v.Platforms = CleanSuggestions([]SuggestedKeyword{{Term: "x", Platforms: v.Platforms}}, nil, allowed, 1)[0].Platforms
		default:
			continue
		}
		seen[term] = true
		out = append(out, v)
	}
	return out
}

func (in KeywordReviewInput) text() string {
	var s strings.Builder
	s.WriteString(in.Brief.text())
	fmt.Fprintf(&s, "\nAllowed platforms for keywords: %s\n", strings.Join(in.Platforms, ", "))
	fmt.Fprintf(&s, "\nKEYWORDS (last %d days):\n", in.Days)
	for _, k := range in.Keywords {
		state := "active"
		if !k.Active {
			state = "paused"
		}
		fmt.Fprintf(&s, "- %q [%s] searches %s: %d posts, %d leads", k.Term, state, strings.Join(k.Platforms, ", "), k.Posts, k.Leads)
		if len(k.ByPlatform) > 0 {
			fmt.Fprintf(&s, " (%s)", strings.Join(k.ByPlatform, "; "))
		}
		s.WriteString("\n")
	}
	if in.Previous != "" {
		fmt.Fprintf(&s, "\nYOUR LAST REVIEW AND WHAT HAPPENED:\n%s\n", in.Previous)
	}
	s.WriteString("\nPOSTS (a sample: leads first, then near misses, low scores and posts the noise filter dropped):\n")
	for i, p := range in.Posts {
		score := "not scored"
		if p.Score > 0 {
			score = fmt.Sprintf("score %.1f", p.Score)
		}
		fmt.Fprintf(&s, "%d. [%s, keyword %q, %s, %s] %s", i+1, p.Platform, p.Keyword, p.Outcome, score, p.Text)
		if p.Reason != "" {
			fmt.Fprintf(&s, "\n   scorer: %s", p.Reason)
		}
		s.WriteString("\n")
	}
	return s.String()
}
