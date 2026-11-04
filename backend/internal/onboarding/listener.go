package onboarding

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"leadecho/internal/monitor"
)

// Goal is one onboarding goal chip; each becomes one listener.
type Goal string

const (
	GoalFindBuyers          Goal = "find_buyers"
	GoalCompetitorSwitchers Goal = "competitor_switchers"
	GoalBrandMentions       Goal = "brand_mentions"
	GoalContentIdeas        Goal = "content_ideas"
)

// AllGoals in display order.
var AllGoals = []Goal{GoalFindBuyers, GoalCompetitorSwitchers, GoalBrandMentions, GoalContentIdeas}

const (
	maxListenerKeywords = 6
	maxSampleText       = 500
	samplesPerListener  = 3
)

// TestablePlatforms have a free public search the builder can live-test.
var TestablePlatforms = map[string]bool{"reddit": true, "hackernews": true, "devto": true}

// ParseGoals validates and dedupes goal strings.
func ParseGoals(in []string) ([]Goal, error) {
	valid := map[Goal]bool{}
	for _, g := range AllGoals {
		valid[g] = true
	}
	seen := map[Goal]bool{}
	var out []Goal
	for _, s := range in {
		g := Goal(strings.TrimSpace(s))
		if !valid[g] {
			return nil, fmt.Errorf("unknown goal %q", s)
		}
		if !seen[g] {
			seen[g] = true
			out = append(out, g)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("at least one goal is required")
	}
	return out, nil
}

// Proposal is a listener the user can confirm, edit or drop.
type Proposal struct {
	Goal      Goal     `json:"goal"`
	Name      string   `json:"name"`
	Keywords  []string `json:"keywords"`
	LooksFor  string   `json:"looks_for"`
	Skips     string   `json:"skips"`
	Platforms []string `json:"platforms"`
}

// TemplateProposal is the deterministic listener for a goal, used when no LLM
// is configured (keyless dev) or the LLM fails.
func TemplateProposal(goal Goal, bp BrandProfile, platforms []string) Proposal {
	product := strings.TrimSpace(bp.Product)
	if product == "" {
		product = "your product"
	}
	cats := bp.Keywords.Category
	category := "tool like " + product
	if len(cats) > 0 {
		category = cats[0]
	}
	problems := bp.Keywords.Problem
	if len(problems) == 0 {
		for _, p := range bp.PainPoints {
			problems = append(problems, p.Problem)
		}
	}
	p := Proposal{Goal: goal, Platforms: nonNil(platforms)}
	switch goal {
	case GoalFindBuyers:
		p.Name = "Buyers looking for a " + category
		p.Keywords = append(append(head(cats, 2), "best "+category), head(problems, 2)...)
		p.LooksFor = fmt.Sprintf("People asking for recommendations for a %s, comparing options, or describing problems like %s.", category, orDefault(head(problems, 2), "the ones "+product+" solves"))
		p.Skips = "Vendors promoting their own product, job posts, news articles and tutorials with no buying need."
	case GoalCompetitorSwitchers:
		p.Name = "Switching from competitors"
		comps := head(bp.Competitors, 3)
		for _, c := range comps {
			p.Keywords = append(p.Keywords, c+" alternative")
		}
		if len(comps) > 0 {
			p.Keywords = append(p.Keywords, "switching from "+comps[0])
		} else {
			p.Keywords = append(p.Keywords, category+" alternative")
		}
		p.LooksFor = fmt.Sprintf("People frustrated with %s (pricing, missing features, support) or asking for alternatives.", orDefault(comps, "their current "+category))
		p.Skips = "Happy customers, competitor announcements and the competitors' own marketing posts."
	case GoalBrandMentions:
		p.Name = product + " mentions"
		p.Keywords = head(orList(bp.Keywords.Brand, []string{product}), maxListenerKeywords)
		p.LooksFor = fmt.Sprintf("Posts and comments that mention %s by name: questions, reviews, complaints and praise.", product)
		p.Skips = fmt.Sprintf("Unrelated uses of the word %q that are not about the product.", product)
	case GoalContentIdeas:
		p.Name = "Content ideas"
		p.Keywords = append(head(problems, 3), category+" tips")
		p.LooksFor = fmt.Sprintf("Recurring questions and discussions about %s that would make good posts or helpful replies.", orDefault(head(problems, 2), category))
		p.Skips = "Promotional posts, job posts and memes."
	}
	p.Keywords = cleanList(p.Keywords, maxListenerKeywords)
	return p
}

// pickSamples returns up to n distinct posts, substantial ones first, newest first.
func pickSamples(posts []monitor.Post, n int) []Sample {
	seen := map[string]bool{}
	var uniq []monitor.Post
	for _, p := range posts {
		if p.URL == "" || seen[p.URL] {
			continue
		}
		seen[p.URL] = true
		uniq = append(uniq, p)
	}
	substantial := func(p monitor.Post) bool { return len(p.Content) >= 40 }
	sort.SliceStable(uniq, func(i, j int) bool {
		if substantial(uniq[i]) != substantial(uniq[j]) {
			return substantial(uniq[i])
		}
		return uniq[i].CreatedAt.After(uniq[j].CreatedAt)
	})
	out := []Sample{}
	for _, p := range head(uniq, n) {
		out = append(out, Sample{
			Platform: p.Platform, URL: p.URL, Title: p.Title, Author: p.Author,
			Text: capLen(p.Content, maxSampleText), CreatedAt: p.CreatedAt,
		})
	}
	return out
}

// Sample is a real post shown under a listener for 👍/👎.
type Sample struct {
	Platform  string    `json:"platform"`
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// head returns a copy of the first n items (a copy, so appending to the
// result never writes into the caller's backing array).
func head[T any](s []T, n int) []T {
	if len(s) > n {
		s = s[:n]
	}
	return append([]T(nil), s...)
}

func orList(a, b []string) []string {
	if len(a) > 0 {
		return a
	}
	return b
}

func orDefault(list []string, def string) string {
	if len(list) == 0 {
		return def
	}
	return strings.Join(list, ", ")
}
