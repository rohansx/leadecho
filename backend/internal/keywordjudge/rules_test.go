package keywordjudge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

// fakeRescorer answers per mention: before (current rules) and after (any
// other rule set). Calls run concurrently, hence the mutex.
type fakeRescorer struct {
	mu            sync.Mutex
	before, after map[string]bool
	fail          map[string]bool
	calls         int
}

func (f *fakeRescorer) RescoreWith(_ context.Context, _, id string, rules []string, _ []ai.ScoringExample) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.fail[id] {
		return false, errors.New("provider down")
	}
	if len(rules) == 1 && rules[0] == "old" {
		return f.before[id], nil
	}
	return f.after[id], nil
}

func post(id, stage string, lead bool) database.ReviewPostRow {
	return database.ReviewPostRow{MentionID: id, Stage: stage, IsLead: lead, Text: "post " + id}
}

var curRules = ScoringRules{Rules: []Rule{{Text: "old"}}}

func TestLabelledPostsSkipsUnscoredAndBadNumbers(t *testing.T) {
	posts := []database.ReviewPostRow{
		post("a", "stage3_classified", true),
		post("b", "stage1_spam_filter", false),
		post("c", "stage3_classified", false),
	}
	got := labelledPosts(posts, []ai.PostLabel{{N: 1, Lead: false}, {N: 2, Lead: true}, {N: 3, Lead: true}, {N: 9, Lead: true}, {N: 1, Lead: true}})
	if len(got) != 2 || got[0].post.MentionID != "a" || got[0].judge || got[1].post.MentionID != "c" || !got[1].judge {
		t.Fatalf("got %+v", got)
	}
}

// A post shown to the scorer as an example must never be in the test set.
func TestSplitHoldsOut(t *testing.T) {
	var ls []labelled
	for i := 0; i < 10; i++ {
		ls = append(ls, labelled{post: post(fmt.Sprint(i), "stage3", false), judge: i < 4})
	}
	ex, test := split(ls)
	shown := map[string]bool{}
	for _, e := range ex {
		shown[e.Content] = true
	}
	ups := 0
	for _, e := range ex {
		if e.Label == "up" {
			ups++
		}
	}
	testLeads := 0
	for _, l := range test {
		if shown[l.post.Text] {
			t.Fatalf("post %s is both an example and a test post", l.post.MentionID)
		}
		if l.judge {
			testLeads++
		}
	}
	if len(ex) != 5 || len(test) != 5 || ups != 2 || testLeads != 2 {
		t.Fatalf("examples %d (leads %d), test %d (leads %d)", len(ex), ups, len(test), testLeads)
	}
}

func evalPosts() []labelled {
	return []labelled{
		{post: post("real1", "", true), judge: true},
		{post: post("real2", "", false), judge: true},
		{post: post("fake1", "", true), judge: false},
		{post: post("fake2", "", true), judge: false},
		{post: post("noise", "", false), judge: false},
	}
}

func TestTestRulesComparesTwoFreshRuns(t *testing.T) {
	rs := &fakeRescorer{
		before: map[string]bool{"real1": true, "fake1": true, "fake2": true}, // current rules: 2 false leads, misses real2
		after:  map[string]bool{"real1": true, "real2": true},                // proposal fixes all three
	}
	tr := testRules(context.Background(), rs, "ws", evalPosts(), curRules, []string{"new"}, nil, time.Now())
	if !tr.Applied || tr.AgreeBefore != 2 || tr.AgreeAfter != 5 || tr.RealLeadsBefore != 1 || tr.RealLeadsAfter != 2 || rs.calls != 10 {
		t.Fatalf("got %+v, calls %d", tr, rs.calls)
	}
}

// Rules that cut noise but also cut a real lead are refused, even when the
// scorer agrees with the review more often overall.
func TestTestRulesRefusesLosingARealLead(t *testing.T) {
	rs := &fakeRescorer{
		before: map[string]bool{"real1": true, "real2": true, "fake1": true, "fake2": true},
		after:  map[string]bool{"real1": true},
	}
	tr := testRules(context.Background(), rs, "ws", evalPosts(), curRules, []string{"new"}, nil, time.Now())
	if tr.Applied || !strings.Contains(tr.Note, "lost 1 real lead") {
		t.Fatalf("got %+v", tr)
	}
}

func TestTestRulesRefusesWorseAgreement(t *testing.T) {
	rs := &fakeRescorer{
		before: map[string]bool{"real1": true, "real2": true},
		after:  map[string]bool{"real1": true, "real2": true, "fake1": true, "fake2": true, "noise": true},
	}
	tr := testRules(context.Background(), rs, "ws", evalPosts(), curRules, []string{"new"}, nil, time.Now())
	if tr.Applied {
		t.Fatalf("agreement fell from %d to %d but it was applied", tr.AgreeBefore, tr.AgreeAfter)
	}
}

// Too few posts, or a provider failing, means no change: never untested.
func TestTestRulesFewPostsAndFailures(t *testing.T) {
	rs := &fakeRescorer{}
	tr := testRules(context.Background(), rs, "ws", evalPosts()[:3], curRules, []string{"new"}, nil, time.Now())
	if tr.Applied || rs.calls != 0 {
		t.Errorf("too few posts: must keep the current rules without model calls, got %+v (calls %d)", tr, rs.calls)
	}
	rs = &fakeRescorer{fail: map[string]bool{"real1": true, "real2": true, "fake1": true}, after: map[string]bool{"real1": true, "real2": true}}
	tr = testRules(context.Background(), rs, "ws", evalPosts(), curRules, []string{"new"}, nil, time.Now())
	if tr.Applied {
		t.Errorf("most posts could not be scored: must keep the current rules, got %+v", tr)
	}
}

// The judge cannot drop the customer's own rules.
func TestMergeProposalKeepsCustomerRules(t *testing.T) {
	cur := ScoringRules{Rules: []Rule{
		{ID: "c", Text: "Students are not leads", Source: "customer"},
		{ID: "j", Text: "Old judge rule", Source: "judge"},
		{ID: "k", Text: "Kept judge rule", Source: "judge"},
	}}
	got := mergeProposal(cur, []string{"kept judge rule", "A new rule"}, time.Now())
	if len(got) != 3 || got[0].ID != "c" || got[1].ID != "k" || got[2].Source != "judge" || got[2].Text != "A new rule" {
		t.Fatalf("got %+v", got)
	}
}

func TestSetCustomerRulesRemembersRemoved(t *testing.T) {
	now := time.Now()
	cur := ScoringRules{Rules: []Rule{
		{ID: "1", Text: "Founders promoting their own app are noise", Source: "judge"},
		{ID: "2", Text: "iOS-only needs score at most 4", Source: "judge"},
	}}
	next := SetCustomerRules(cur, []string{"founders promoting their own app are noise", "  Indian buyers matter most  "}, now)
	if len(next.Rules) != 2 || next.Rules[0].ID != "1" || next.Rules[1].Source != "customer" || next.Rules[1].Text != "Indian buyers matter most" {
		t.Fatalf("rules: %+v", next.Rules)
	}
	if len(next.Removed) != 1 || next.Removed[0] != "iOS-only needs score at most 4" {
		t.Fatalf("removed: %+v", next.Removed)
	}
	if got := ai.CleanRules([]string{"iOS-only needs score at most 4", "new"}, next.Removed); len(got) != 1 || got[0] != "new" {
		t.Fatalf("a removed rule must not come back: %v", got)
	}
}
