package monitor

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
	"github.com/rs/zerolog"

	"leadecho/internal/ai"
	"leadecho/internal/billing"
	"leadecho/internal/database"
	"leadecho/internal/llm"
	"leadecho/internal/workflow"
)

// fakeScorer: posts saying "spamme" fail the noise filter, "refuse" is
// refused by the classifier, anything else is a strong buyer lead (which a
// normal scan would make a leads row, draft and workflow run for). down makes
// every classification fail (a provider outage); onCall runs on each call.
type fakeScorer struct {
	classified atomic.Int32
	down       atomic.Bool
	onCall     func(n int32)
}

func (f *fakeScorer) FilterMention(_ context.Context, _, _, content, _, _ string) (*ai.FilterResult, error) {
	return &ai.FilterResult{Pass: !strings.Contains(content, "spamme"), Reason: "noise"}, nil
}

func (f *fakeScorer) EmbedTexts(context.Context, string, llm.Task, []string) ([]pgvector.Vector, error) {
	return nil, errors.New("no embedder in this test")
}

func (f *fakeScorer) ClassifyIntent(_ context.Context, _, _, content, _ string, _ ai.ScoringContext) (*ai.ClassifyResult, error) {
	n := f.classified.Add(1)
	if f.onCall != nil {
		f.onCall(n)
	}
	switch {
	case f.down.Load():
		return nil, errors.New("503 Service Unavailable")
	case strings.Contains(content, "refuse"):
		return nil, ai.ErrRefused
	case strings.Contains(content, "meh"): // still a buyer, no longer a lead
		return &ai.ClassifyResult{Intent: "general", RelevanceScore: 3, AuthorRole: "buyer"}, nil
	}
	return &ai.ClassifyResult{Intent: "buy_signal", RelevanceScore: 8, ConversionProbability: 0.7, AuthorRole: "buyer", IntentTags: []string{"buying_intent"}}, nil
}

type countingWorkflows struct{ n atomic.Int32 }

func (c *countingWorkflows) Run(context.Context, workflow.Lead) { c.n.Add(1) }

const rescoreBody = "we are looking for a tool that does exactly this, any suggestions?"

type rescoreFixture struct {
	t    *testing.T
	ctx  context.Context
	pool *pgxpool.Pool
	q    *database.Queries
	ws   string
}

func newRescoreFixture(t *testing.T) *rescoreFixture {
	dsn := os.Getenv("LEADECHO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEADECHO_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var ws string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug, credit_balance) VALUES ('rl-'||gen_random_uuid(), 'rl', 'rl-'||gen_random_uuid(), 100) RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), `DELETE FROM workspaces WHERE id = $1`, ws) })
	return &rescoreFixture{t: t, ctx: ctx, pool: pool, q: database.New(pool), ws: ws}
}

func (f *rescoreFixture) add(pid, content, status, intent, scoring, platformMeta string, score float64) string {
	var id string
	if err := f.pool.QueryRow(f.ctx, `
INSERT INTO mentions (workspace_id, platform, platform_id, url, content, status, intent, scoring_metadata, platform_metadata, relevance_score, engagement_metrics, keyword_matches)
VALUES ($1, 'reddit', $2, 'https://x/'||$2, $3, $4::mention_status, $5::intent_type, $6::jsonb, $7::jsonb, $8, '{}', '{}')
RETURNING id::text`, f.ws, pid, content, status, intent, scoring, platformMeta, score).Scan(&id); err != nil {
		f.t.Fatalf("insert %s: %v", pid, err)
	}
	return id
}

func (f *rescoreFixture) monitor(s *fakeScorer) (*Monitor, *countingWorkflows, *countingDrafter) {
	wf, drafter := &countingWorkflows{}, &countingDrafter{}
	m := New(f.q, zerolog.Nop(), "", s, nil, "", nil, false, false, false, false, nil)
	m.SetWorkflows(wf)
	m.SetAutoDrafter(drafter)
	m.credits = billing.New(f.q, true)
	return m, wf, drafter
}

func (f *rescoreFixture) due(cutoff time.Time) int {
	rows, err := f.q.ListLeadsToRescore(f.ctx, f.ws, cutoff, nil, 100)
	if err != nil {
		f.t.Fatal(err)
	}
	return len(rows)
}

type rescoredRow struct {
	Intent *string
	Score  *float64
	Meta   map[string]any
}

func (f *rescoreFixture) read(id string) rescoredRow {
	var r rescoredRow
	if err := f.pool.QueryRow(f.ctx, `SELECT intent::text, relevance_score, scoring_metadata FROM mentions WHERE id = $1`, id).Scan(&r.Intent, &r.Score, &r.Meta); err != nil {
		f.t.Fatal(err)
	}
	return r
}

// DB-backed: the leads re-score picks exactly the shown post leads scored
// before the request, changes their verdicts and nothing else (no leads row,
// draft, workflow or credit), keeps the review flags of posts that stay leads
// and clears them on posts that do not, stamps every post it tried and finds
// nothing the second time.
func TestRescoreLeadsDB(t *testing.T) {
	f := newRescoreFixture(t)
	cutoff := time.Now().UTC()
	later := cutoff.Add(time.Minute).Format(time.RFC3339Nano)

	flagged := f.add("flagged", rescoreBody, "new", "complaint", `{"stage":"stage3_classified","needs_escalation":true,"escalation_reason":"pricing question"}`, `{}`, 7)
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO leads (workspace_id, mention_id, stage) VALUES ($1, $2, 'prospect')`, f.ws, flagged); err != nil {
		t.Fatal(err)
	}
	noLead := f.add("nolead", rescoreBody, "reviewed", "comparison", `{"stage":"stage3_classified"}`, `{}`, 6.5)
	dropped := f.add("dropped", rescoreBody+" spamme", "new", "buy_signal", `{"stage":"stage3_classified","needs_escalation":true,"escalation_reason":"x","rescreen":{"why":"kept"}}`, `{}`, 9)
	demoted := f.add("demoted", rescoreBody+" meh", "new", "buy_signal", `{"needs_escalation":true,"escalation_reason":"y"}`, `{}`, 7)
	refused := f.add("refused", rescoreBody+" refuse", "new", "recommendation_ask", `{}`, `{}`, 7)
	// Not candidates: a company signal, an archived lead, a post below the
	// line, a post that is not a buying intent, one scored after the cutoff,
	// one refused after the cutoff (final).
	f.add("signal", rescoreBody, "new", "buy_signal", `{}`, `{"signal":"hiring"}`, 8)
	f.add("archived", rescoreBody, "archived", "buy_signal", `{}`, `{}`, 8)
	f.add("low", rescoreBody, "new", "buy_signal", `{}`, `{}`, 5)
	f.add("general", rescoreBody, "new", "general", `{}`, `{}`, 8)
	f.add("fresh", rescoreBody, "new", "buy_signal", `{"scored_at":"`+later+`"}`, `{}`, 8)
	f.add("refusedbefore", rescoreBody, "new", "buy_signal", `{"scored_at":"`+later+`","rescore_error":"stage3_refused"}`, `{}`, 8)
	// Due although scored after the cutoff: the provider failed on it.
	failedBefore := f.add("failedbefore", rescoreBody, "new", "buy_signal", `{"scored_at":"`+later+`","rescore_error":"stage3_classification_error"}`, `{}`, 8)

	got, err := f.q.ListLeadsToRescore(f.ctx, f.ws, cutoff, []string{noLead}, 50)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{flagged: true, dropped: true, demoted: true, refused: true, failedBefore: true}
	if len(got) != len(want) {
		t.Fatalf("candidates = %d %v, want the 5 due post leads other than the one already tried", len(got), got)
	}
	for _, r := range got {
		if !want[r.ID] {
			t.Fatalf("unexpected candidate %s", r.ID)
		}
	}

	scorer := &fakeScorer{}
	m, wf, drafter := f.monitor(scorer)
	note, complete := m.rescoreLeads(f.ctx, f.ws, cutoff)
	if !complete || !strings.HasPrefix(note, "5 leads got a new verdict") || !strings.Contains(note, "1 were refused") {
		t.Fatalf("note = %q complete = %v", note, complete)
	}

	stamped := func(id string) rescoredRow {
		r := f.read(id)
		if r.Meta["scored_at"] == nil {
			t.Errorf("%s: scored_at not stamped: %v", id, r.Meta)
		}
		return r
	}
	if r := stamped(flagged); r.Score == nil || *r.Score != 8 || r.Meta["needs_escalation"] != true || r.Meta["escalation_reason"] != "pricing question" {
		t.Errorf("a lead that stays a lead keeps its review flag: %+v", r)
	}
	if r := stamped(noLead); r.Score == nil || *r.Score != 8 {
		t.Errorf("no-lead post not re-scored: %+v", r)
	}
	if r := stamped(dropped); r.Intent != nil || r.Score != nil || r.Meta["filtered"] != true || r.Meta["rescreen"] == nil || r.Meta["needs_escalation"] != nil || r.Meta["escalation_reason"] != nil {
		t.Errorf("filtered post loses its verdict and review flag, keeps rescreen: %+v", r)
	}
	if r := stamped(demoted); r.Score == nil || *r.Score != 3 || r.Meta["needs_escalation"] != nil {
		t.Errorf("demoted post leaves needs review: %+v", r)
	}
	if r := stamped(refused); r.Score == nil || *r.Score != 7 || r.Meta["rescore_error"] != "stage3_refused" {
		t.Errorf("refused post keeps its verdict and is marked: %+v", r)
	}
	if r := stamped(failedBefore); r.Meta["rescore_error"] != nil || *r.Score != 8 {
		t.Errorf("a post that failed before is re-scored and loses its error: %+v", r)
	}

	var leads, replies, balance int
	if err := f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM leads WHERE workspace_id = $1), (SELECT count(*) FROM replies WHERE workspace_id = $1), (SELECT credit_balance FROM workspaces WHERE id = $1)`, f.ws).Scan(&leads, &replies, &balance); err != nil {
		t.Fatal(err)
	}
	if leads != 1 || replies != 0 || wf.n.Load() != 0 || len(drafter.calls) != 0 {
		t.Errorf("leads=%d replies=%d workflows=%d drafts=%d, want only the existing lead and nothing else", leads, replies, wf.n.Load(), len(drafter.calls))
	}
	if balance != 100 {
		t.Errorf("credit balance = %d, want 100 (the customer is not charged)", balance)
	}

	// Idempotent: the same request finds nothing and calls no model.
	calls := scorer.classified.Load()
	if note, complete := m.rescoreLeads(f.ctx, f.ws, cutoff); !complete || note != "no shown lead was scored before this request" {
		t.Fatalf("second run note = %q complete = %v", note, complete)
	}
	if scorer.classified.Load() != calls {
		t.Errorf("second run classified %d more posts", scorer.classified.Load()-calls)
	}

	// One leads row per post: the second insert is the duplicate qualifyAsLead ignores.
	_, err = f.q.CreateLead(f.ctx, database.CreateLeadParams{WorkspaceID: f.ws, MentionID: pgUUID(flagged), Stage: database.LeadStageProspect, Tags: []string{}, Metadata: []byte(`{}`)})
	if !isDuplicateError(err) {
		t.Errorf("second lead for one post: err = %v, want a duplicate", err)
	}
}

// DB-backed: a run cut short (the 45-minute limit, a deploy) is not
// complete, and the next request carries on: it scores only what is left.
func TestRescoreLeadsResumeDB(t *testing.T) {
	f := newRescoreFixture(t)
	defer func(b int32) { rescoreLeadsBatch = b }(rescoreLeadsBatch)
	rescoreLeadsBatch = 2
	for _, pid := range []string{"a", "b", "c", "d", "e", "g"} {
		f.add(pid, rescoreBody, "new", "buy_signal", `{}`, `{}`, 8)
	}
	first, err := NewManualScan(ScanRescoreLeads, "op", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	// Cut short as the second batch starts.
	ctx, cancel := context.WithCancel(f.ctx)
	m, _, _ := f.monitor(&fakeScorer{onCall: func(n int32) {
		if n == 3 {
			cancel()
		}
	}})
	note, complete := m.rescoreLeads(ctx, f.ws, first.ScoredBefore())
	if complete || !strings.Contains(note, "ask again to carry on") {
		t.Fatalf("cut-short run: note = %q complete = %v", note, complete)
	}
	first.Complete = complete
	left := f.due(first.ScoredBefore())
	if left < 2 || left > 4 {
		t.Fatalf("after the cut-short run %d leads are due, want the 2 scored before the cut done", left)
	}

	// A new request an hour later continues the first.
	second, _ := NewManualScan(ScanRescoreLeads, "op", time.Now().UTC().Add(time.Hour))
	second = second.ContinueFrom(first)
	if !second.ScoredBefore().Equal(first.ScoredBefore()) {
		t.Fatalf("the follow-up request did not keep the run's cutoff")
	}
	scorer := &fakeScorer{}
	m, _, _ = f.monitor(scorer)
	if note, complete := m.rescoreLeads(f.ctx, f.ws, second.ScoredBefore()); !complete || int(scorer.classified.Load()) != left {
		t.Fatalf("follow-up run classified %d leads (%q), want only the %d left", scorer.classified.Load(), note, left)
	}

	// After a complete run, a new request starts afresh.
	second.Complete = true
	third, _ := NewManualScan(ScanRescoreLeads, "op", time.Now().UTC().Add(2*time.Hour))
	if third.ContinueFrom(second).ScoredBefore() != third.RequestedAt {
		t.Fatalf("a request after a complete run must not reuse its cutoff")
	}
	if other, _ := NewManualScan(ScanRescore, "op", time.Now()); !other.ContinueFrom(first).Since.IsZero() {
		t.Fatalf("only a leads re-score continues a leads re-score")
	}
}

// DB-backed: when the provider is down the run stops after a batch instead
// of reporting success, and the failed leads are due again: the next request
// retries only them.
func TestRescoreLeadsOutageDB(t *testing.T) {
	f := newRescoreFixture(t)
	defer func(b int32) { rescoreLeadsBatch = b }(rescoreLeadsBatch)
	rescoreLeadsBatch = 2
	var ids []string
	for _, pid := range []string{"a", "b", "c", "d"} {
		ids = append(ids, f.add(pid, rescoreBody, "new", "buy_signal", `{}`, `{}`, 7))
	}
	cutoff := time.Now().UTC()
	scorer := &fakeScorer{}
	scorer.down.Store(true)
	m, _, _ := f.monitor(scorer)
	note, complete := m.rescoreLeads(f.ctx, f.ws, cutoff)
	if complete || !strings.Contains(note, "AI provider failed on 2 of 2") || scorer.classified.Load() != 2 {
		t.Fatalf("outage: note = %q complete = %v calls = %d", note, complete, scorer.classified.Load())
	}
	for _, id := range ids {
		if r := f.read(id); *r.Score != 7 {
			t.Fatalf("a failed call changed a verdict: %+v", r)
		}
	}
	if n := f.due(cutoff); n != 4 {
		t.Fatalf("%d leads due after the outage, want all 4 (2 failed, 2 not tried)", n)
	}

	scorer = &fakeScorer{}
	m, _, _ = f.monitor(scorer)
	if note, complete := m.rescoreLeads(f.ctx, f.ws, cutoff); !complete || note != "4 leads got a new verdict" {
		t.Fatalf("retry: note = %q complete = %v", note, complete)
	}
}

// The manual scan accepts the new kind.
func TestNewManualScanRescoreLeads(t *testing.T) {
	if _, err := NewManualScan(ScanRescoreLeads, "op", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManualScan("rescore_everything", "op", time.Now()); !errors.Is(err, ErrScanKind) {
		t.Fatalf("unknown kind: err = %v", err)
	}
}
