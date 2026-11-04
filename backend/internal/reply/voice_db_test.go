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

	t.Run("no profile, onboarding url: tracked link inserted", func(t *testing.T) {
		ws := newWS()
		if _, err := q.CreateOnboardingAnalysis(ctx, database.CreateOnboardingAnalysisParams{WorkspaceID: ws, SourceUrl: "https://acme.dev", Analysis: []byte("{}"), Status: "completed"}); err != nil {
			t.Fatal(err)
		}
		res, err := d.DraftForMention(ctx, ws, newMention(ws))
		if err != nil {
			t.Fatal(err)
		}
		if !res.Reply.UtmLinkID.Valid || !strings.Contains(res.Reply.Content, "https://app.test/r/") {
			t.Fatalf("want tracked link, got %+v", res.Reply)
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
