package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// CustomerSegment is one buyer group from the product analysis. The fields
// after WhyTheyBuy describe the segment well enough to search for companies
// in it and check each one: profiles saved before they existed have only
// the name, and are not used for prospecting.
type CustomerSegment struct {
	Name       string `json:"name"`
	WhyTheyBuy string `json:"why_they_buy"`
	Pain       string `json:"pain,omitempty"` // the problem, in the buyer's words
	// Criteria are facts visible on a company's website or profile that put
	// it in the segment; NegativeCriteria look similar but rule it out.
	Criteria         []string `json:"criteria,omitempty"`
	NegativeCriteria []string `json:"negative_criteria,omitempty"`
	ExampleCompanies []string `json:"example_companies,omitempty"`
	BuyerTitles      string   `json:"buyer_titles,omitempty"` // "Practice Director or Managing Partner"
	Country          string   `json:"country,omitempty"`      // ISO 3166-1 alpha-2; "" = any
}

// Prospectable reports whether the segment says enough to search for
// companies in it.
func (s CustomerSegment) Prospectable() bool {
	return s.Name != "" && len(s.Criteria) > 0 && s.BuyerTitles != ""
}

// PainPointDetail is a pain point with a short explanation.
type PainPointDetail struct {
	Problem     string `json:"problem"`
	Explanation string `json:"explanation"`
}

// KeywordGroups splits monitoring keywords by kind.
type KeywordGroups struct {
	Brand    []string `json:"brand"`
	Product  []string `json:"product"`
	Category []string `json:"category"`
	Problem  []string `json:"problem"`
}

// ListenerPrompt is the context for generating one onboarding listener.
type ListenerPrompt struct {
	Goal        string
	Product     string
	Overview    string
	Audience    string
	Competitors []string
	PainPoints  []string
	Keywords    []string
}

// ListenerDraft is the LLM's proposal for one listener.
type ListenerDraft struct {
	Name     string   `json:"name"`
	Keywords []string `json:"keywords"`
	LooksFor string   `json:"looks_for"`
	Skips    string   `json:"skips"`
}

const listenerSystemPrompt = `You configure a social-listening "listener" that finds posts on Reddit, Hacker News and Dev.to for one goal.

Goals:
- find_buyers: people actively looking for a tool like this product or describing the problem it solves
- competitor_switchers: people unhappy with a named competitor or asking for alternatives
- brand_mentions: anyone mentioning the product by name
- content_ideas: recurring questions and discussions the product team could write about

Return ONLY valid JSON (no markdown, no code fences):
- "name": short listener name (max 6 words)
- "keywords": 3-6 short search phrases (2-4 words each) that real people type in posts, not marketing copy, ordered best first. For find_buyers and competitor_switchers use phrases a BUYER writes when they need this ("recommend a <category>", "<competitor> alternative", "switching from <competitor>", "<category> for <their situation>"), not the name of a topic: a topic phrase ("technical interviews", "email marketing") mostly finds people chatting about the subject, not people who need the product
- "looks_for": one or two plain-English sentences describing posts to keep
- "skips": one plain-English sentence describing posts to ignore (vendors self-promoting, job posts, news, unrelated meanings)`

func (lp ListenerPrompt) userText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Goal: %s\nProduct: %s\n", lp.Goal, lp.Product)
	if lp.Overview != "" {
		fmt.Fprintf(&b, "What it does: %s\n", lp.Overview)
	}
	if lp.Audience != "" {
		fmt.Fprintf(&b, "Audience: %s\n", lp.Audience)
	}
	if len(lp.Competitors) > 0 {
		fmt.Fprintf(&b, "Competitors: %s\n", strings.Join(lp.Competitors, ", "))
	}
	if len(lp.PainPoints) > 0 {
		fmt.Fprintf(&b, "Pain points: %s\n", strings.Join(lp.PainPoints, "; "))
	}
	if len(lp.Keywords) > 0 {
		fmt.Fprintf(&b, "Known keywords: %s\n", strings.Join(lp.Keywords, ", "))
	}
	return b.String()
}

// GenerateListener asks the model for keywords and Looks-for / Skips for one goal.
func GenerateListener(ctx context.Context, p Provider, lp ListenerPrompt) (*ListenerDraft, error) {
	result, err := callChat(ctx, p, []chatMessage{
		{Role: "system", Content: listenerSystemPrompt},
		{Role: "user", Content: lp.userText()},
	}, 0.3)
	if err != nil {
		return nil, err
	}
	var d ListenerDraft
	if err := json.Unmarshal([]byte(stripCodeFences(result)), &d); err != nil {
		return nil, fmt.Errorf("parse listener draft: %w", err)
	}
	if len(d.Keywords) == 0 {
		return nil, errors.New("listener draft has no keywords")
	}
	return &d, nil
}
