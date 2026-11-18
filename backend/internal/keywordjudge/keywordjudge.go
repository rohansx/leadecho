// Package keywordjudge keeps a workspace's keywords good. Suggest fills a
// keyword list out; Review is the judge: it reads what the keywords found
// (leads, near misses, posts the filters dropped) as the founder would, and
// stores what to add, pause or move. Each review also sees what became of
// the last one's advice (added keywords and how they did, dismissed ones),
// so the loop corrects itself: suggestion, customer decision, results, next
// review.
package keywordjudge

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/billing"
	"leadecho/internal/database"
)

// Window is how far back a review reads.
const Window = 14 * 24 * time.Hour

// Interval is how often a Pro workspace is reviewed on its own.
const Interval = 7 * 24 * time.Hour

// TargetKeywords is how many active keywords a Pro workspace should have
// before the app stops offering to generate more.
const TargetKeywords = 30

// Suggester writes keywords; Reviewer runs the judge (the LLM router).
type Suggester interface {
	SuggestKeywords(ctx context.Context, workspaceID string, req ai.KeywordSuggestRequest) ([]ai.SuggestedKeyword, error)
}
type Reviewer interface {
	ReviewKeywords(ctx context.Context, workspaceID string, in ai.KeywordReviewInput) (*ai.KeywordReview, error)
}

// KeywordAI is both jobs (the LLM router implements it).
type KeywordAI interface {
	Suggester
	Reviewer
}

// ErrNoProfile: the workspace has not finished onboarding.
var ErrNoProfile = errors.New("the workspace has not finished onboarding")

// Suggestion is one change the judge proposes. Kind is add, pause,
// platforms (move an existing keyword to other platforms) or keep.
type Suggestion struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Term      string   `json:"term"`
	Platforms []string `json:"platforms,omitempty"`
	Angle     string   `json:"angle,omitempty"`
	Why       string   `json:"why"`
}

// Stored is the review kept under workspaces.settings.keyword_review.
type Stored struct {
	GeneratedAt time.Time         `json:"generated_at"`
	By          string            `json:"by"`
	Days        int               `json:"days"`
	Posts       int               `json:"posts"`
	Leads       int               `json:"leads"`
	Summary     string            `json:"summary"`
	LeadQuality string            `json:"lead_quality"`
	Suggestions []Suggestion      `json:"suggestions"`
	Scoring     []ai.ScoringNote  `json:"scoring"`
	Decisions   map[string]string `json:"decisions"`
}

func suggestionID(kind, term string) string {
	sum := sha1.Sum([]byte(kind + "\x00" + strings.ToLower(strings.TrimSpace(term))))
	return hex.EncodeToString(sum[:6])
}

// Brief turns the stored brand profile into what the AI jobs read.
func Brief(bp database.BrandProfile) ai.ProductBrief {
	var segs []ai.CustomerSegment
	_ = json.Unmarshal(bp.CustomerSegments, &segs) // a bad blob just means no segments
	var pains []ai.PainPointDetail
	_ = json.Unmarshal(bp.PainPoints, &pains)
	b := ai.ProductBrief{
		Product: bp.Product, Overview: bp.Overview, Positioning: bp.Positioning,
		Audience: bp.TargetAudience, Features: first(bp.Features, 8), Competitors: first(bp.Competitors, 10),
	}
	for _, s := range first(segs, 6) {
		line := strings.TrimSpace(s.Name)
		if s.Pain != "" {
			line += ": " + s.Pain
		}
		if s.Country != "" {
			line += " (" + s.Country + ")"
		}
		b.Segments = append(b.Segments, line)
	}
	for _, p := range first(pains, 10) {
		b.PainPoints = append(b.PainPoints, p.Problem)
	}
	return b
}

// Platforms are the sources a suggestion may use: all of them, or only the
// free ones when the workspace cannot use paid data.
func Platforms(paidLocked bool) []string {
	out := []string{}
	for _, p := range ai.KeywordPlatforms {
		if paidLocked && billing.PaidPlatforms[p] {
			continue
		}
		out = append(out, p)
	}
	return out
}

// Suggest asks for count new keywords for the workspace.
func Suggest(ctx context.Context, q *database.Queries, s Suggester, wsID string, paidLocked bool, count int) ([]ai.SuggestedKeyword, error) {
	bp, err := q.GetBrandProfile(ctx, wsID)
	if err != nil || bp.Product == "" {
		return nil, ErrNoProfile
	}
	kws, err := q.ListKeywords(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("list keywords: %w", err)
	}
	existing := make([]string, 0, len(kws))
	for _, k := range kws {
		existing = append(existing, k.Term)
	}
	return s.SuggestKeywords(ctx, wsID, ai.KeywordSuggestRequest{
		Brief: Brief(bp), Existing: existing, Platforms: Platforms(paidLocked), Count: count,
	})
}

// Review runs the judge for one workspace and stores its report. by is who
// asked ("weekly", or an email). With rs set it also tests the scoring
// rules the judge proposes and keeps them if they pass (see rules.go).
func Review(ctx context.Context, q *database.Queries, r Reviewer, rs Rescorer, wsID, by string, paidLocked bool, now time.Time) (*Stored, error) {
	bp, err := q.GetBrandProfile(ctx, wsID)
	if err != nil || bp.Product == "" {
		return nil, ErrNoProfile
	}
	since := now.Add(-Window)
	kws, err := q.ListKeywords(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("list keywords: %w", err)
	}
	yield, err := q.ListKeywordYield(ctx, wsID, since)
	if err != nil {
		return nil, fmt.Errorf("keyword yield: %w", err)
	}
	posts, err := q.SampleReviewPosts(ctx, wsID, since)
	if err != nil {
		return nil, fmt.Errorf("sample posts: %w", err)
	}
	stats := keywordStats(kws, yield)
	var prev *Stored
	if raw, err := q.GetKeywordReview(ctx, wsID); err == nil && len(raw) > 0 {
		var p Stored
		if json.Unmarshal(raw, &p) == nil {
			prev = &p
		}
	}
	rules, err := LoadRules(ctx, q, wsID)
	if err != nil {
		return nil, err
	}
	in := ai.KeywordReviewInput{
		Brief: Brief(bp), Keywords: stats, Posts: reviewPosts(posts),
		Platforms: Platforms(paidLocked), Previous: previousText(prev, stats), Days: int(Window.Hours() / 24),
		Rules: rules.Texts(), Removed: rules.Removed,
	}
	rev, err := r.ReviewKeywords(ctx, wsID, in)
	if err != nil {
		return nil, fmt.Errorf("keyword review: %w", err)
	}
	st := toStored(rev, stats, by, now)
	raw, err := json.Marshal(st)
	if err != nil {
		return nil, err
	}
	if err := q.SaveKeywordReview(ctx, wsID, raw); err != nil {
		return nil, fmt.Errorf("save keyword review: %w", err)
	}
	if rs != nil {
		if _, err := updateRules(ctx, q, rs, wsID, rules, rev, posts, now); err != nil {
			return &st, fmt.Errorf("scoring rules: %w", err)
		}
	}
	return &st, nil
}

func toStored(rev *ai.KeywordReview, stats []ai.KeywordStat, by string, now time.Time) Stored {
	st := Stored{
		GeneratedAt: now, By: by, Days: int(Window.Hours() / 24),
		Summary: rev.Summary, LeadQuality: rev.LeadQuality, Scoring: rev.Scoring,
		Decisions: map[string]string{}, Suggestions: []Suggestion{},
	}
	if st.Scoring == nil {
		st.Scoring = []ai.ScoringNote{}
	}
	for _, s := range stats {
		st.Posts += s.Posts
		st.Leads += s.Leads
	}
	for _, a := range rev.Add {
		st.Suggestions = append(st.Suggestions, Suggestion{ID: suggestionID("add", a.Term), Kind: "add", Term: a.Term, Platforms: a.Platforms, Angle: a.Angle, Why: a.Why})
	}
	for _, v := range rev.Verdicts {
		st.Suggestions = append(st.Suggestions, Suggestion{ID: suggestionID(v.Action, v.Term), Kind: v.Action, Term: v.Term, Platforms: v.Platforms, Why: v.Why})
	}
	return st
}

// keywordStats totals the yield rows per keyword, every keyword included
// (one with no posts is a finding too).
func keywordStats(kws []database.ListKeywordsRow, rows []database.KeywordYield) []ai.KeywordStat {
	type agg struct {
		posts, leads int
		by           []string
	}
	per := map[string]*agg{}
	for _, r := range rows {
		a := per[r.KeywordID]
		if a == nil {
			a = &agg{}
			per[r.KeywordID] = a
		}
		a.posts += r.Posts
		a.leads += r.Leads
		a.by = append(a.by, fmt.Sprintf("%s %d posts, %d judged, %d leads", r.Platform, r.Posts, r.Judged, r.Leads))
	}
	out := make([]ai.KeywordStat, 0, len(kws))
	for _, k := range kws {
		s := ai.KeywordStat{Term: k.Term, Active: k.IsActive, Platforms: k.Platforms}
		if a := per[k.ID]; a != nil {
			sort.Strings(a.by)
			s.Posts, s.Leads, s.ByPlatform = a.posts, a.leads, a.by
		}
		out = append(out, s)
	}
	return out
}

func reviewPosts(rows []database.ReviewPostRow) []ai.ReviewPost {
	out := make([]ai.ReviewPost, 0, len(rows))
	for _, r := range rows {
		outcome := "scored, not a lead"
		switch {
		case r.IsLead:
			outcome = "LEAD"
		case strings.HasPrefix(r.Stage, "stage3"):
		case r.Stage != "":
			outcome = "dropped before scoring (" + r.Stage + ")"
		}
		out = append(out, ai.ReviewPost{Platform: r.Platform, Keyword: r.Keyword, Score: r.Score, Outcome: outcome, Text: r.Text, Reason: r.Reason})
	}
	return out
}

// previousText tells the judge what became of its last advice.
func previousText(prev *Stored, stats []ai.KeywordStat) string {
	if prev == nil || len(prev.Suggestions) == 0 {
		return ""
	}
	byTerm := map[string]ai.KeywordStat{}
	for _, s := range stats {
		byTerm[strings.ToLower(s.Term)] = s
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Reviewed %s. Summary then: %s\n", prev.GeneratedAt.Format("2006-01-02"), prev.Summary)
	for _, s := range prev.Suggestions {
		if s.Kind == "keep" {
			continue
		}
		decision := prev.Decisions[s.ID]
		if decision == "" {
			decision = "no decision yet"
		}
		line := fmt.Sprintf("- %s %q: %s", s.Kind, s.Term, decision)
		if st, ok := byTerm[strings.ToLower(s.Term)]; ok && s.Kind == "add" {
			line += fmt.Sprintf("; since then %d posts, %d leads", st.Posts, st.Leads)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func first[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}
