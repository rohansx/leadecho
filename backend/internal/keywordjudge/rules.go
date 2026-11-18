package keywordjudge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

// Learned scoring rules: the judge teaches the scorer. Each review proposes
// the workspace's full rule set and labels the posts it read; the proposal
// is tested on held-out posts (never the ones used as examples): each is
// scored with the current rules and with the proposal through the same path,
// and the proposal is kept only if the scorer then agrees with the judge at
// least as often and keeps every real lead it kept before. Half the judge's
// labelled posts become few-shot examples (the customer's own inbox labels
// come first). Too few posts to test means no change.
// Rules are shown on the Keywords page, where the customer can edit them.

const (
	maxEvalPosts     = 12 // each is scored twice: 24 model calls at most
	minEvalPosts     = 4
	maxJudgeExamples = ai.MaxScoringExamples
	testConcurrency  = 4
	testTimeout      = 5 * time.Minute
)

// Rule is one learned scoring rule.
type Rule struct {
	ID      string    `json:"id"`
	Text    string    `json:"text"`
	Source  string    `json:"source"` // judge | customer
	AddedAt time.Time `json:"added_at"`
}

// RuleTest is how the last proposal did against the judge's labels.
type RuleTest struct {
	At       time.Time `json:"at"`
	Proposed []string  `json:"proposed"`
	Posts    int       `json:"posts"`
	// How many of the posts the scorer called the same as the judge,
	// with the current rules (Before) and with the proposal (After).
	AgreeBefore int `json:"agree_before"`
	AgreeAfter  int `json:"agree_after"`
	// Of the posts the judge calls real leads, how many each set keeps.
	RealLeads       int    `json:"real_leads"`
	RealLeadsBefore int    `json:"real_leads_before"`
	RealLeadsAfter  int    `json:"real_leads_after"`
	Applied         bool   `json:"applied"`
	Note            string `json:"note"`
}

// ScoringRules is stored under workspaces.settings.scoring_rules.
type ScoringRules struct {
	Rules     []Rule              `json:"rules"`
	Removed   []string            `json:"removed"`
	Examples  []ai.ScoringExample `json:"examples"`
	LastTest  *RuleTest           `json:"last_test,omitempty"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// Texts are the rule sentences, as the scorer reads them.
func (s ScoringRules) Texts() []string {
	out := make([]string, 0, len(s.Rules))
	for _, r := range s.Rules {
		out = append(out, r.Text)
	}
	return out
}

// LoadRules reads the workspace's rules (empty when it has none).
func LoadRules(ctx context.Context, q *database.Queries, wsID string) (ScoringRules, error) {
	var s ScoringRules
	raw, err := q.GetWorkspaceSettingsKey(ctx, wsID, database.ScoringRulesKey)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return ScoringRules{}, fmt.Errorf("parse scoring rules: %w", err)
	}
	return s, nil
}

// SaveRules stores the workspace's rules.
func SaveRules(ctx context.Context, q *database.Queries, wsID string, s ScoringRules) error {
	if s.Rules == nil {
		s.Rules = []Rule{}
	}
	if s.Removed == nil {
		s.Removed = []string{}
	}
	if s.Examples == nil {
		s.Examples = []ai.ScoringExample{}
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return q.SaveScoringRules(ctx, wsID, raw)
}

// SetCustomerRules replaces the rules with the customer's edit. Rules they
// dropped are remembered so the judge does not propose them again; new or
// reworded ones are marked as theirs.
func SetCustomerRules(cur ScoringRules, texts []string, now time.Time) ScoringRules {
	texts = ai.CleanRules(texts, nil)
	keep := map[string]bool{}
	for _, t := range texts {
		keep[norm(t)] = true
	}
	old := map[string]Rule{}
	for _, r := range cur.Rules {
		old[norm(r.Text)] = r
		if !keep[norm(r.Text)] {
			cur.Removed = appendUnique(cur.Removed, r.Text, 30)
		}
	}
	next := make([]Rule, 0, len(texts))
	for _, t := range texts {
		if r, ok := old[norm(t)]; ok {
			next = append(next, r)
			continue
		}
		next = append(next, Rule{ID: suggestionID("rule", t), Text: t, Source: "customer", AddedAt: now})
	}
	cur.Rules, cur.UpdatedAt = next, now
	return cur
}

// Rescorer scores one stored post again with candidate rules and examples
// and says whether it would now be a lead (the monitor implements it).
type Rescorer interface {
	RescoreWith(ctx context.Context, wsID, mentionID string, rules []string, examples []ai.ScoringExample) (bool, error)
}

// labelled is a scored post the judge gave a verdict on.
type labelled struct {
	post  database.ReviewPostRow
	judge bool // a lead per the judge
}

// labelledPosts pairs the judge's labels with the scored posts it read
// (the numbers are 1-based positions in the sample).
func labelledPosts(posts []database.ReviewPostRow, labels []ai.PostLabel) []labelled {
	var out []labelled
	seen := map[int]bool{}
	for _, l := range labels {
		i := l.N - 1
		if i < 0 || i >= len(posts) || seen[i] || !strings.HasPrefix(posts[i].Stage, "stage3") {
			continue
		}
		seen[i] = true
		out = append(out, labelled{post: posts[i], judge: l.Lead})
	}
	return out
}

// split holds the labelled posts out: every other post (leads and non-leads
// alternating separately, so both halves get some of each) is a few-shot
// example, the rest are the test set. A post is never both, or the test
// would grade the scorer on answers it was shown.
func split(ls []labelled) (examples []ai.ScoringExample, test []labelled) {
	var leads, others []labelled
	for _, l := range ls {
		if l.judge {
			leads = append(leads, l)
		} else {
			others = append(others, l)
		}
	}
	for _, group := range [][]labelled{leads, others} {
		for i, l := range group {
			if i%2 == 1 && len(examples) < maxJudgeExamples {
				label, note := "down", "review: not a lead"
				if l.judge {
					label, note = "up", "review: a real lead"
				}
				examples = append(examples, ai.ScoringExample{Content: l.post.Text, Label: label, Note: note})
				continue
			}
			test = append(test, l)
		}
	}
	// Every real lead in the test half is kept (the guard against losing
	// leads must see them); the other posts fill up to the cap.
	if len(test) > maxEvalPosts {
		test = test[:maxEvalPosts]
	}
	return examples, test
}

// testRules scores the test set twice through the same path, with the
// current rules and examples and with the proposal, and decides. Both runs
// see the same prompt apart from the change, so the comparison is fair.
func testRules(ctx context.Context, rs Rescorer, wsID string, set []labelled, cur ScoringRules, rules []string, examples []ai.ScoringExample, now time.Time) RuleTest {
	t := RuleTest{At: now, Proposed: rules, Posts: len(set)}
	if len(set) < minEvalPosts {
		t.Note = fmt.Sprintf("only %d scored posts to test on, too few to trust; kept the current rules", len(set))
		return t
	}
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	type verdict struct{ before, after, ok bool }
	out := make([]verdict, len(set))
	var wg sync.WaitGroup
	sem := make(chan struct{}, testConcurrency)
	for i, l := range set {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			b, errB := rs.RescoreWith(ctx, wsID, id, cur.Texts(), cur.Examples)
			a, errA := rs.RescoreWith(ctx, wsID, id, rules, examples)
			out[i] = verdict{before: b, after: a, ok: errB == nil && errA == nil}
		}(i, l.post.MentionID)
	}
	wg.Wait()
	scored := 0
	for i, l := range set {
		v := out[i]
		if !v.ok {
			continue
		}
		scored++
		if v.before == l.judge {
			t.AgreeBefore++
		}
		if v.after == l.judge {
			t.AgreeAfter++
		}
		if l.judge {
			t.RealLeads++
			if v.before {
				t.RealLeadsBefore++
			}
			if v.after {
				t.RealLeadsAfter++
			}
		}
	}
	t.Posts = scored
	if scored < minEvalPosts || scored*2 < len(set) {
		t.Note = fmt.Sprintf("the test could score only %d of %d posts; kept the current rules", scored, len(set))
		return t
	}
	t.Applied = t.AgreeAfter >= t.AgreeBefore && t.RealLeadsAfter >= t.RealLeadsBefore
	switch {
	case t.Applied:
		t.Note = fmt.Sprintf("applied: on %d held-out posts the scorer matched the review %d times with the new rules and %d with the old, and kept %d of %d real leads", t.Posts, t.AgreeAfter, t.AgreeBefore, t.RealLeadsAfter, t.RealLeads)
	case t.RealLeadsAfter < t.RealLeadsBefore:
		t.Note = fmt.Sprintf("not applied: it would have lost %d real leads", t.RealLeadsBefore-t.RealLeadsAfter)
	default:
		t.Note = fmt.Sprintf("not applied: on %d held-out posts the scorer would match the review %d times instead of %d", t.Posts, t.AgreeAfter, t.AgreeBefore)
	}
	return t
}

// updateRules runs after a review: test the proposed rules (with the
// judge's examples) and keep them if they pass. The rules are read again
// just before saving, so an edit the customer made while the test ran is
// kept, and their own rules survive any proposal.
func updateRules(ctx context.Context, q *database.Queries, rs Rescorer, wsID string, cur ScoringRules, rev *ai.KeywordReview, posts []database.ReviewPostRow, now time.Time) (ScoringRules, error) {
	proposed := ai.CleanRules(rev.Rules, cur.Removed)
	if len(proposed) == 0 || sameRules(proposed, cur.Texts()) {
		return cur, nil
	}
	if last := cur.LastTest; last != nil && !last.Applied && sameRules(proposed, last.Proposed) {
		return cur, nil // the same proposal failed last time: do not pay to test it again
	}
	examples, set := split(labelledPosts(posts, rev.Labels))
	t := testRules(ctx, rs, wsID, set, cur, proposed, examples, now)

	fresh, err := LoadRules(ctx, q, wsID)
	if err != nil {
		return cur, err
	}
	fresh.LastTest, fresh.UpdatedAt = &t, now
	if t.Applied {
		fresh.Rules = mergeProposal(fresh, ai.CleanRules(proposed, fresh.Removed), now)
		fresh.Examples = examples
	}
	return fresh, SaveRules(ctx, q, wsID, fresh)
}

// mergeProposal is the applied rule set: the proposal, keeping the ids of
// rules that already existed, plus the customer's own rules (the judge may
// not drop them), up to the cap.
func mergeProposal(cur ScoringRules, proposed []string, now time.Time) []Rule {
	old := map[string]Rule{}
	for _, r := range cur.Rules {
		old[norm(r.Text)] = r
	}
	next := []Rule{}
	in := map[string]bool{}
	for _, r := range cur.Rules {
		if r.Source == "customer" && len(next) < ai.MaxScoringRules {
			next = append(next, r)
			in[norm(r.Text)] = true
		}
	}
	for _, p := range proposed {
		if in[norm(p)] || len(next) == ai.MaxScoringRules {
			continue
		}
		in[norm(p)] = true
		if r, ok := old[norm(p)]; ok {
			next = append(next, r)
			continue
		}
		next = append(next, Rule{ID: suggestionID("rule", p), Text: p, Source: "judge", AddedAt: now})
	}
	return next
}

func sameRules(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]bool{}
	for _, x := range a {
		set[norm(x)] = true
	}
	for _, x := range b {
		if !set[norm(x)] {
			return false
		}
	}
	return true
}

func norm(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

func appendUnique(list []string, s string, max int) []string {
	for _, x := range list {
		if norm(x) == norm(s) {
			return list
		}
	}
	list = append(list, s)
	if len(list) > max {
		list = list[len(list)-max:]
	}
	return list
}
