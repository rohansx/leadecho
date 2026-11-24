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
// normal scan would make a leads row, draft and workflow run for).
type fakeScorer struct{ classified atomic.Int32 }

func (f *fakeScorer) FilterMention(_ context.Context, _, _, content, _, _ string) (*ai.FilterResult, error) {
	return &ai.FilterResult{Pass: !strings.Contains(content, "spamme"), Reason: "noise"}, nil
}

func (f *fakeScorer) EmbedTexts(context.Context, string, llm.Task, []string) ([]pgvector.Vector, error) {
	return nil, errors.New("no embedder in this test")
}

func (f *fakeScorer) ClassifyIntent(_ context.Context, _, _, content, _ string, _ ai.ScoringContext) (*ai.ClassifyResult, error) {
	f.classified.Add(1)
	if strings.Contains(content, "refuse") {
		return nil, ai.ErrRefused
	}
	return &ai.ClassifyResult{Intent: "buy_signal", RelevanceScore: 8, ConversionProbability: 0.7, AuthorRole: "buyer", IntentTags: []string{"buying_intent"}}, nil
}

type countingWorkflows struct{ n atomic.Int32 }

func (c *countingWorkflows) Run(context.Context, workflow.Lead) { c.n.Add(1) }

// DB-backed: the leads re-score picks exactly the shown post leads scored
// before the request, changes their verdicts and nothing else (no leads row,
// draft, workflow or credit), keeps the review flags, stamps every post it
// tried (dropped and refused ones too) and finds nothing the second time.
func TestRescoreLeadsDB(t *testing.T) {
	dsn := os.Getenv("LEADECHO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEADECHO_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var ws string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug, credit_balance) VALUES ('rl-'||gen_random_uuid(), 'rl', 'rl-'||gen_random_uuid(), 100) RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)

	add := func(pid, content, status, intent, scoring, platformMeta string, score float64) string {
		var id string
		if err := pool.QueryRow(ctx, `
INSERT INTO mentions (workspace_id, platform, platform_id, url, content, status, intent, scoring_metadata, platform_metadata, relevance_score, engagement_metrics, keyword_matches)
VALUES ($1, 'reddit', $2, 'https://x/'||$2, $3, $4::mention_status, $5::intent_type, $6::jsonb, $7::jsonb, $8, '{}', '{}')
RETURNING id::text`, ws, pid, content, status, intent, scoring, platformMeta, score).Scan(&id); err != nil {
			t.Fatalf("insert %s: %v", pid, err)
		}
		return id
	}
	const body = "we are looking for a tool that does exactly this, any suggestions?"
	requestedAt := time.Now().UTC()
	later := requestedAt.Add(time.Minute).Format(time.RFC3339Nano)

	flagged := add("flagged", body, "new", "complaint", `{"stage":"stage3_classified","needs_escalation":true,"escalation_reason":"pricing question"}`, `{}`, 7)
	if _, err := pool.Exec(ctx, `INSERT INTO leads (workspace_id, mention_id, stage) VALUES ($1, $2, 'prospect')`, ws, flagged); err != nil {
		t.Fatal(err)
	}
	noLead := add("nolead", body, "reviewed", "comparison", `{"stage":"stage3_classified"}`, `{}`, 6.5)
	dropped := add("dropped", body+" spamme", "new", "buy_signal", `{"stage":"stage3_classified","rescreen":{"why":"kept"}}`, `{}`, 9)
	refused := add("refused", body+" refuse", "new", "recommendation_ask", `{}`, `{}`, 7)
	// Not candidates: a company signal, an archived lead, a post below the
	// line, a post that is not a buying intent, one scored after the request.
	add("signal", body, "new", "buy_signal", `{}`, `{"signal":"hiring"}`, 8)
	add("archived", body, "archived", "buy_signal", `{}`, `{}`, 8)
	add("low", body, "new", "buy_signal", `{}`, `{}`, 5)
	add("general", body, "new", "general", `{}`, `{}`, 8)
	add("fresh", body, "new", "buy_signal", `{"scored_at":"`+later+`"}`, `{}`, 8)

	q := database.New(pool)
	got, err := q.ListLeadsToRescore(ctx, ws, requestedAt, 50)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{flagged: true, noLead: true, dropped: true, refused: true}
	if len(got) != len(want) {
		t.Fatalf("candidates = %d %v, want the 4 shown post leads", len(got), got)
	}
	for _, r := range got {
		if !want[r.ID] {
			t.Fatalf("unexpected candidate %s", r.ID)
		}
	}

	scorer, wf, drafter := &fakeScorer{}, &countingWorkflows{}, &countingDrafter{}
	m := New(q, zerolog.Nop(), "", scorer, nil, "", nil, false, false, false, false, nil)
	m.SetWorkflows(wf)
	m.SetAutoDrafter(drafter)
	m.credits = billing.New(q, true)

	if note := m.rescoreLeads(ctx, ws, requestedAt); note != "scored 4 leads again" {
		t.Fatalf("note = %q", note)
	}

	type row struct {
		Intent *string
		Score  *float64
		Meta   map[string]any
	}
	read := func(id string) row {
		var r row
		if err := pool.QueryRow(ctx, `SELECT intent::text, relevance_score, scoring_metadata FROM mentions WHERE id = $1`, id).Scan(&r.Intent, &r.Score, &r.Meta); err != nil {
			t.Fatal(err)
		}
		if r.Meta["scored_at"] == nil {
			t.Errorf("%s: scored_at not stamped: %v", id, r.Meta)
		}
		return r
	}
	if r := read(flagged); r.Score == nil || *r.Score != 8 || r.Meta["needs_escalation"] != true || r.Meta["escalation_reason"] != "pricing question" || r.Meta["stage"] != "stage3_classified" {
		t.Errorf("re-scored lead lost its new score or review flag: %+v", r)
	}
	if r := read(noLead); r.Score == nil || *r.Score != 8 {
		t.Errorf("no-lead post not re-scored: %+v", r)
	}
	if r := read(dropped); r.Intent != nil || r.Score != nil || r.Meta["filtered"] != true || r.Meta["rescreen"] == nil {
		t.Errorf("filtered post should lose its verdict and keep rescreen: %+v", r)
	}
	if r := read(refused); r.Score == nil || *r.Score != 7 || r.Meta["rescore_error"] != "stage3_refused" {
		t.Errorf("refused post should keep its verdict and be marked: %+v", r)
	}

	var leads, replies, balance int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM leads WHERE workspace_id = $1), (SELECT count(*) FROM replies WHERE workspace_id = $1), (SELECT credit_balance FROM workspaces WHERE id = $1)`, ws).Scan(&leads, &replies, &balance); err != nil {
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
	if note := m.rescoreLeads(ctx, ws, requestedAt); note != "no shown lead was scored before this request" {
		t.Fatalf("second run note = %q", note)
	}
	if scorer.classified.Load() != calls {
		t.Errorf("second run classified %d more posts", scorer.classified.Load()-calls)
	}

	// One leads row per post: the second insert is the duplicate qualifyAsLead ignores.
	_, err = q.CreateLead(ctx, database.CreateLeadParams{WorkspaceID: ws, MentionID: pgUUID(flagged), Stage: database.LeadStageProspect, Tags: []string{}, Metadata: []byte(`{}`)})
	if !isDuplicateError(err) {
		t.Errorf("second lead for one post: err = %v, want a duplicate", err)
	}
}

// The manual scan accepts the new kind; the old re-score kind is unchanged.
func TestNewManualScanRescoreLeads(t *testing.T) {
	if _, err := NewManualScan(ScanRescoreLeads, "op", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManualScan("rescore_everything", "op", time.Now()); !errors.Is(err, ErrScanKind) {
		t.Fatalf("unknown kind: err = %v", err)
	}
}
