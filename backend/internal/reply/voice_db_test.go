package reply

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

// captureGen records the drafting options and returns a canned reply.
type captureGen struct {
	opts  ai.DraftReplyOptions
	reply string
}

func (c *captureGen) PreFilterForReply(context.Context, string, string, string, string, string) (*ai.PreFilterResult, error) {
	return &ai.PreFilterResult{ShouldReply: true, AwarenessLevel: "solution_aware"}, nil
}

func (c *captureGen) DraftReplyEnhanced(_ context.Context, _ string, opts ai.DraftReplyOptions) (*ai.EnhancedDraftResult, error) {
	c.opts = opts
	return &ai.EnhancedDraftResult{Reply: c.reply, Tone: "helpful", TemplateStyle: opts.TemplateStyle}, nil
}

// noReplyGen says no post is worth a reply.
type noReplyGen struct{ *captureGen }

func (noReplyGen) PreFilterForReply(context.Context, string, string, string, string, string) (*ai.PreFilterResult, error) {
	return &ai.PreFilterResult{ShouldReply: false, Reason: "an announcement"}, nil
}

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated database.
func TestDraftWithVoiceDB(t *testing.T) {
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
	q := database.New(pool)

	newWS := func() string {
		n := time.Now().UnixNano()
		ws, err := q.CreateWorkspace(ctx, database.CreateWorkspaceParams{ClerkOrgID: fmt.Sprint("voice-", n), Name: "Voice", Slug: fmt.Sprint("voice-", n)})
		if err != nil {
			t.Fatal(err)
		}
		return ws.ID
	}
	newMention := func(ws string) string {
		m, err := q.CreateMention(ctx, database.CreateMentionParams{
			WorkspaceID: ws, Platform: "reddit", PlatformID: fmt.Sprint(time.Now().UnixNano()),
			Url: "https://reddit.com/r/x/1", Content: "Which queue should I use?", Status: database.MentionStatusNew,
			PlatformMetadata: []byte("{}"), EngagementMetrics: []byte("{}"), KeywordMatches: []string{},
		})
		if err != nil {
			t.Fatal(err)
		}
		return m.ID
	}
	gen := &captureGen{reply: "Use a durable queue with retries."}
	d := NewDrafter(q, gen, nil, nil)
	d.SetShortLinkBase("https://app.test")

	t.Run("no profile, no url: derived voice, no link, no gating", func(t *testing.T) {
		ws := newWS()
		res, err := d.DraftForMention(ctx, ws, newMention(ws))
		if err != nil || !res.ShouldReply {
			t.Fatalf("draft: %v %+v", err, res)
		}
		if !strings.Contains(gen.opts.Voice, "move the reader") {
			t.Fatalf("derived voice missing: %q", gen.opts.Voice)
		}
		if res.Reply.UtmLinkID.Valid || strings.Contains(res.Reply.Content, "/r/") {
			t.Fatalf("unexpected link without destination: %+v", res.Reply)
		}
	})

	t.Run("onboarding url: no link in a public reply unless the workspace switched it on", func(t *testing.T) {
		ws := newWS()
		if _, err := q.CreateOnboardingAnalysis(ctx, database.CreateOnboardingAnalysisParams{WorkspaceID: ws, SourceUrl: "https://acme.dev", Analysis: []byte("{}"), Status: "completed"}); err != nil {
			t.Fatal(err)
		}
		res, err := d.DraftForMention(ctx, ws, newMention(ws))
		if err != nil {
			t.Fatal(err)
		}
		if res.Reply.UtmLinkID.Valid || strings.Contains(res.Reply.Content, "/r/") || strings.Contains(res.Reply.Content, "http") {
			t.Fatalf("a public reply must carry no link by default, got %+v", res.Reply)
		}

		if err := q.SetDraftSettings(ctx, ws, database.DraftSettings{PublicReplyLinks: true}); err != nil {
			t.Fatal(err)
		}
		res, err = d.DraftForMention(ctx, ws, newMention(ws))
		if err != nil {
			t.Fatal(err)
		}
		if !res.Reply.UtmLinkID.Valid || !strings.Contains(res.Reply.Content, "https://app.test/r/") {
			t.Fatalf("opted in: want tracked link, got %+v", res.Reply)
		}
	})

	t.Run("the draft model is told what the product is and what it replaces", func(t *testing.T) {
		ws := newWS()
		if _, err := pool.Exec(ctx, `INSERT INTO brand_profiles (workspace_id, overview, product, positioning) VALUES ($1, 'Live-system assessments.', 'Utkrusht', 'Real work instead of take-homes.')`, ws); err != nil {
			t.Fatal(err)
		}
		if _, err := d.DraftForMention(ctx, ws, newMention(ws)); err != nil {
			t.Fatal(err)
		}
		if gen.opts.ProductDescription != "Live-system assessments." || gen.opts.Positioning != "Real work instead of take-homes." {
			t.Fatalf("positioning not passed to the draft: %+v", gen.opts)
		}
	})

	t.Run("auto-draft: free, capped per day, once per lead, never flags", func(t *testing.T) {
		ws := newWS()
		first := newMention(ws)
		if ok, err := d.AutoDraft(ctx, ws, first); err != nil || !ok {
			t.Fatalf("first auto-draft: %v %v", ok, err)
		}
		if ok, err := d.AutoDraft(ctx, ws, first); err != nil || ok {
			t.Fatalf("a lead that has a draft must not get another: %v %v", ok, err)
		}
		for i := 1; i < database.DefaultAutoDraftDaily; i++ {
			if ok, err := d.AutoDraft(ctx, ws, newMention(ws)); err != nil || !ok {
				t.Fatalf("auto-draft %d: %v %v", i+1, ok, err)
			}
		}
		over := newMention(ws)
		if ok, err := d.AutoDraft(ctx, ws, over); err != nil || ok {
			t.Fatalf("over the daily cap: %v %v", ok, err)
		}
		if rs, _ := q.ListRepliesByMention(ctx, database.ListRepliesByMentionParams{MentionID: over, WorkspaceID: ws}); len(rs) != 0 {
			t.Fatalf("over the cap nothing is written, got %d", len(rs))
		}
		// The customer can still ask for it.
		if res, err := d.DraftForMention(ctx, ws, over); err != nil || !res.ShouldReply {
			t.Fatalf("manual draft after the cap: %v", err)
		}

		// Switched off; and a "not worth a reply" verdict gives the slot back
		// and leaves the lead unflagged.
		off, zero := newWS(), 0
		if err := q.SetDraftSettings(ctx, off, database.DraftSettings{AutoDraftDaily: &zero}); err != nil {
			t.Fatal(err)
		}
		if ok, _ := d.AutoDraft(ctx, off, newMention(off)); ok {
			t.Fatal("auto-drafts are off for this workspace")
		}
		one, quiet := 1, newWS()
		if err := q.SetDraftSettings(ctx, quiet, database.DraftSettings{AutoDraftDaily: &one}); err != nil {
			t.Fatal(err)
		}
		no := NewDrafter(q, noReplyGen{gen}, nil, nil)
		skipped := newMention(quiet)
		if ok, err := no.AutoDraft(ctx, quiet, skipped); err != nil || ok {
			t.Fatalf("not worth a reply: %v %v", ok, err)
		}
		m, err := q.GetMention(ctx, database.GetMentionParams{ID: skipped, WorkspaceID: quiet})
		if err != nil || strings.Contains(string(m.ScoringMetadata), "needs_escalation") {
			t.Fatalf("an unasked-for draft must not flag the lead: %s %v", m.ScoringMetadata, err)
		}
		if ok, err := d.AutoDraft(ctx, quiet, newMention(quiet)); err != nil || !ok {
			t.Fatalf("the unused slot must be free again: %v %v", ok, err)
		}
	})

	t.Run("explicit profile: example used, brand_plug none skips link", func(t *testing.T) {
		ws := newWS()
		p, err := q.CreateResponseProfile(ctx, database.CreateResponseProfileParams{
			WorkspaceID: ws, Name: "Quiet", Goal: "build_relationship", Tone: 10, Length: "short", Assertiveness: 10,
			BrandPlug: "none", DestinationUrl: "https://acme.dev", Examples: []byte(`{"reddit":"honestly just use sqs lol"}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !p.IsDefault {
			t.Fatal("first profile should be default")
		}
		res, err := d.DraftWithProfile(ctx, ws, newMention(ws), p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if gen.opts.StyleExample != "honestly just use sqs lol" || !strings.Contains(gen.opts.Voice, "Do NOT mention") {
			t.Fatalf("voice not applied: %+v", gen.opts)
		}
		if res.Reply.UtmLinkID.Valid {
			t.Fatal("brand_plug none must not link")
		}
	})

	t.Run("profile from another workspace is not found", func(t *testing.T) {
		wsA, wsB := newWS(), newWS()
		p, err := EnsureDefaultResponseProfile(ctx, q, wsA)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.DraftWithProfile(ctx, wsB, newMention(wsB), p.ID); !errors.Is(err, ErrResponseProfileNotFound) {
			t.Fatalf("want ErrResponseProfileNotFound, got %v", err)
		}
		if n, _ := q.SetDefaultResponseProfile(ctx, database.SetDefaultResponseProfileParams{ID: p.ID, WorkspaceID: wsB}); n != 0 {
			t.Fatalf("cross-workspace set-default touched %d rows", n)
		}
	})

	t.Run("ensure default is idempotent and set default flips", func(t *testing.T) {
		ws := newWS()
		a, err := EnsureDefaultResponseProfile(ctx, q, ws)
		if err != nil {
			t.Fatal(err)
		}
		again, err := EnsureDefaultResponseProfile(ctx, q, ws)
		if err != nil || again.ID != a.ID {
			t.Fatalf("ensure not idempotent: %v %s vs %s", err, again.ID, a.ID)
		}
		if a.Goal != "drive_conversion" || a.BrandPlug != "low" || !a.IsDefault {
			t.Fatalf("bad default %+v", a)
		}
		b, err := q.CreateResponseProfile(ctx, database.CreateResponseProfileParams{
			WorkspaceID: ws, Name: "B", Goal: "support", Tone: 50, Length: "medium", Assertiveness: 50, BrandPlug: "low", Examples: []byte("{}"),
		})
		if err != nil || b.IsDefault {
			t.Fatalf("second profile must not be default: %v %+v", err, b)
		}
		if n, err := q.SetDefaultResponseProfile(ctx, database.SetDefaultResponseProfileParams{ID: b.ID, WorkspaceID: ws}); err != nil || n != 2 {
			t.Fatalf("set default: n=%d err=%v", n, err)
		}
		got, _ := q.GetPreferredResponseProfile(ctx, ws)
		if got.ID != b.ID {
			t.Fatalf("preferred = %s, want %s", got.ID, b.ID)
		}
	})
}
