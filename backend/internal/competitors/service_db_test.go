package competitors

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/ai"
	"leadecho/internal/database"
	"leadecho/internal/monitor"
)

type fakeModel struct {
	mu      sync.Mutex
	prompts []string
	answer  func(system, user string) (string, error)
}

func (f *fakeModel) Chat(_ context.Context, _ string, msgs []ai.ChatMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prompts = append(f.prompts, msgs[1].Content)
	return f.answer(msgs[0].Content, msgs[1].Content)
}

type fakePosts struct {
	items []Item
	err   error
}

func (f fakePosts) CompanyPosts(context.Context, string, string, string) ([]Item, error) {
	return f.items, f.err
}

type site struct {
	mu    sync.Mutex
	pages map[string]string
}

func (s *site) set(u, body string) { s.mu.Lock(); s.pages[u] = body; s.mu.Unlock() }
func (s *site) fetch(_ context.Context, u string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.pages[u]; ok {
		return b, nil
	}
	return "", errors.New("status 404")
}

// testDB returns a migrated database and a fresh workspace, or skips.
func testDB(t *testing.T) (*pgxpool.Pool, *database.Queries, string) {
	t.Helper()
	dsn := os.Getenv("LEADECHO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEADECHO_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := database.New(pool)
	slug := fmt.Sprintf("cw-%d", time.Now().UnixNano())
	ws, err := q.CreateWorkspace(ctx, database.CreateWorkspaceParams{ClerkOrgID: slug, Name: "Competitor watch test", Slug: slug})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM workspaces WHERE id = $1", ws.ID)
		pool.Close()
	})
	return pool, q, ws.ID
}

func brand(t *testing.T, q *database.Queries, ws string, competitors ...string) {
	t.Helper()
	if competitors == nil {
		competitors = []string{}
	}
	if _, err := q.UpsertBrandProfile(context.Background(), database.UpsertBrandProfileParams{
		WorkspaceID: ws, Product: "Mine", Overview: "Hiring tests", CustomerSegments: []byte("[]"), Competitors: competitors,
		PainPoints: []byte("[]"), Keywords: []byte("{}"), Features: []string{}, SourceUrl: "https://mine.com",
	}); err != nil {
		t.Fatal(err)
	}
}

// makeDue pushes every source of the workspace past the 24h check interval.
func makeDue(t *testing.T, pool *pgxpool.Pool, ws string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "UPDATE competitor_sources SET last_checked_at = NOW() - INTERVAL '2 days' WHERE workspace_id = $1", ws); err != nil {
		t.Fatal(err)
	}
}

func pricingPage(price string) string {
	return page(append(longLines(6), "Starter "+price+" per month")...)
}

func TestSetupPollMovesAlertDB(t *testing.T) {
	pool, q, ws := testDB(t)
	ctx := context.Background()
	brand(t, q, ws, "Acme HR", "Ghost Co")

	home := `<html><body>` + strings.Join(longLines(6), " ") + `<a href="/pricing">Pricing</a><a href="https://x.com/acmehr">X</a></body></html>`
	web := &site{pages: map[string]string{"https://acmehr.com": home, "https://acmehr.com/pricing": pricingPage("$29")}}
	model := &fakeModel{answer: func(system, user string) (string, error) {
		if strings.Contains(system, "short briefing") {
			return "Acme is raising prices.", nil
		}
		return `{"moves":[{"item":1,"kind":"pricing","title":"Starter raised to $49","what":"Starter went from $29 to $49.","theme":"t1","impact":"You are cheaper now.","action":"Say so."}]}`, nil
	}}
	var alerts []string
	svc := &Service{
		Q: q, Fetch: web.fetch, Model: model, Logger: zerolog.Nop(),
		Search: func(_ context.Context, name string) ([]monitor.ExaWebResult, error) {
			if name == "Acme HR" {
				return []monitor.ExaWebResult{{Title: "Acme HR | Hiring", URL: "https://acmehr.com"}}, nil
			}
			return []monitor.ExaWebResult{{Title: "Top 10 tools", URL: "https://listicle.io/top"}}, nil
		},
		Posts: fakePosts{err: ErrLocked},
		Themes: func(context.Context, string) ([]Theme, error) {
			return []Theme{{ID: "t1", Title: "Cheaper plans", Coverage: "covered"}}, nil
		},
		Alert: func(_ context.Context, _, competitor string, a Alert) {
			alerts = append(alerts, competitor+": "+a.Title)
		},
	}

	// Round 1: setup creates Acme with its sources; Ghost Co has no site.
	svc.Poll(ctx)
	comps, _ := q.ListCompetitors(ctx, ws)
	if len(comps) != 1 || comps[0].Name != "Acme HR" || comps[0].Website != "https://acmehr.com" {
		t.Fatalf("competitors: %+v", comps)
	}
	setup, err := q.GetCompetitorSetup(ctx, ws)
	if err != nil || len(setup.Unresolved) != 1 || setup.Unresolved[0] != "Ghost Co" {
		t.Fatalf("setup: %+v %v", setup, err)
	}
	srcs, _ := q.ListCompetitorSources(ctx, ws)
	byKind := map[string]database.CompetitorSource{}
	for _, s := range srcs {
		byKind[s.Kind] = s
	}
	if len(srcs) != 3 || byKind[KindPricing].Url != "https://acmehr.com/pricing" || byKind[KindX].Url != "https://x.com/acmehr" {
		t.Fatalf("sources: %+v", srcs)
	}
	// The same round already checked them: a baseline, no items, no model call.
	if byKind[KindPricing].LastHash == "" || byKind[KindLanding].LastHash == "" {
		t.Fatalf("first check must store a baseline: %+v", srcs)
	}
	if x := byKind[KindX]; x.Failures != 0 || x.Broken || !x.LastCheckedAt.Valid {
		t.Fatalf("a locked source waits without counting a failure: %+v", x)
	}
	if len(model.prompts) != 0 {
		t.Fatalf("a baseline must not reach the model, got %d calls", len(model.prompts))
	}

	// Round 2: the pricing page changed.
	web.set("https://acmehr.com/pricing", pricingPage("$49"))
	makeDue(t, pool, ws)
	svc.Poll(ctx)

	moves, _ := q.ListCompetitorMoves(ctx, database.ListCompetitorMovesParams{WorkspaceID: ws, Since: time.Now().Add(-time.Hour)})
	if len(moves) != 1 {
		t.Fatalf("moves: %+v", moves)
	}
	m := moves[0]
	if m.Kind != "pricing" || m.Importance != "high" || m.ThemeID != "t1" || m.Impact != "You are cheaper now." ||
		len(m.Urls) != 1 || m.Urls[0] != "https://acmehr.com/pricing" || !m.AlertedAt.Valid {
		t.Fatalf("move: %+v", m)
	}
	if !strings.Contains(model.prompts[0], "Starter $49 per month") || !strings.Contains(model.prompts[0], "t1 | covered | Cheaper plans") {
		t.Fatalf("the model must see the changed lines and the themes:\n%s", model.prompts[0])
	}
	comps, _ = q.ListCompetitors(ctx, ws)
	if comps[0].Summary != "Acme is raising prices." {
		t.Fatalf("summary: %q", comps[0].Summary)
	}
	if len(alerts) != 1 || alerts[0] != "Acme HR: Starter raised to $49" {
		t.Fatalf("alerts: %v", alerts)
	}

	// Round 3: nothing changed. No new move, no second alert, no model call.
	calls := len(model.prompts)
	makeDue(t, pool, ws)
	svc.Poll(ctx)
	moves, _ = q.ListCompetitorMoves(ctx, database.ListCompetitorMovesParams{WorkspaceID: ws, Since: time.Now().Add(-time.Hour)})
	if len(moves) != 1 || len(alerts) != 1 || len(model.prompts) != calls {
		t.Fatalf("an unchanged page must do nothing: %d moves, %d alerts, %d calls", len(moves), len(alerts), len(model.prompts)-calls)
	}
}

func TestModelFailureKeepsItemsForRetryDB(t *testing.T) {
	pool, q, ws := testDB(t)
	ctx := context.Background()
	web := &site{pages: map[string]string{"https://acme.com": pricingPage("$29")}}
	fail := true
	model := &fakeModel{answer: func(system, _ string) (string, error) {
		if fail {
			return "sorry, no JSON here", nil
		}
		if strings.Contains(system, "short briefing") {
			return "s", nil
		}
		return `{"moves":[{"item":1,"kind":"positioning","title":"New headline"}]}`, nil
	}}
	svc := &Service{Q: q, Fetch: web.fetch, Model: model, Logger: zerolog.Nop()}
	c, err := q.CreateCompetitor(ctx, database.CreateCompetitorParams{WorkspaceID: ws, Name: "Acme", Website: "https://acme.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateCompetitorSource(ctx, database.CreateCompetitorSourceParams{CompetitorID: c.ID, WorkspaceID: ws, Kind: KindLanding, Url: "https://acme.com"}); err != nil {
		t.Fatal(err)
	}
	svc.Poll(ctx) // baseline
	web.set("https://acme.com", pricingPage("$49"))
	makeDue(t, pool, ws)
	svc.Poll(ctx) // change found, model answer unusable

	count := func() (moves, waiting int) {
		pool.QueryRow(ctx, "SELECT COUNT(*) FROM competitor_moves WHERE workspace_id = $1", ws).Scan(&moves)
		pool.QueryRow(ctx, "SELECT COUNT(*) FROM competitor_items WHERE workspace_id = $1 AND processed_at IS NULL", ws).Scan(&waiting)
		return
	}
	if moves, waiting := count(); moves != 0 || waiting != 1 {
		t.Fatalf("after a bad answer: %d moves, %d waiting", moves, waiting)
	}
	fail = false
	svc.Poll(ctx)
	if moves, waiting := count(); moves != 1 || waiting != 0 {
		t.Fatalf("after the retry: %d moves, %d waiting", moves, waiting)
	}
}

func TestFailuresBreakASourceAndFeedsDoNotReplayDB(t *testing.T) {
	pool, q, ws := testDB(t)
	ctx := context.Background()
	web := &site{pages: map[string]string{"https://acme.com/blog": rssSample}}
	model := &fakeModel{answer: func(string, string) (string, error) { return `{"moves":[]}`, nil }}
	svc := &Service{Q: q, Fetch: web.fetch, Model: model, Logger: zerolog.Nop()}
	c, _ := q.CreateCompetitor(ctx, database.CreateCompetitorParams{WorkspaceID: ws, Name: "Acme", Website: "https://acme.com"})
	for kind, u := range map[string]string{KindBlog: "https://acme.com/blog", KindPricing: "https://acme.com/pricing"} {
		if _, err := q.CreateCompetitorSource(ctx, database.CreateCompetitorSourceParams{CompetitorID: c.ID, WorkspaceID: ws, Kind: kind, Url: u}); err != nil {
			t.Fatal(err)
		}
	}
	// The pricing page is rendered in the browser: no text ever arrives.
	web.set("https://acme.com/pricing", `<html><body><div id="app"></div><script>boot()</script></body></html>`)

	for i := 0; i < 5; i++ {
		svc.Poll(ctx)
		makeDue(t, pool, ws)
	}
	var failures int
	var broken bool
	pool.QueryRow(ctx, "SELECT failures, broken FROM competitor_sources WHERE workspace_id = $1 AND kind = $2", ws, KindPricing).Scan(&failures, &broken)
	if failures != 5 || !broken {
		t.Fatalf("five empty fetches must break the source: failures=%d broken=%v", failures, broken)
	}
	svc.Poll(ctx)
	pool.QueryRow(ctx, "SELECT failures FROM competitor_sources WHERE workspace_id = $1 AND kind = $2", ws, KindPricing).Scan(&failures)
	if failures != 5 {
		t.Fatalf("a broken source is not polled again, failures=%d", failures)
	}

	// The feed was read six times: its two entries are stored once, as seen.
	var items, waiting int
	pool.QueryRow(ctx, "SELECT COUNT(*), COUNT(*) FILTER (WHERE processed_at IS NULL) FROM competitor_items WHERE workspace_id = $1", ws).Scan(&items, &waiting)
	if items != 2 || waiting != 0 || len(model.prompts) != 0 {
		t.Fatalf("feed history must not become moves: %d items, %d waiting, %d model calls", items, waiting, len(model.prompts))
	}

	// A new entry dated today is news; one re-published with an old date is not.
	fresh := time.Now().UTC().Format(time.RFC1123Z)
	web.set("https://acme.com/blog", strings.Replace(rssSample, "<item>",
		"<item><title>New today</title><guid>new-1</guid><pubDate>"+fresh+"</pubDate></item>"+
			"<item><title>Old, re-keyed</title><guid>old-rekeyed</guid><pubDate>Mon, 02 Jan 2023 09:00:00 +0000</pubDate></item><item>", 1))
	makeDue(t, pool, ws)
	svc.Poll(ctx)
	if len(model.prompts) != 1 || !strings.Contains(model.prompts[0], "New today") || strings.Contains(model.prompts[0], "Old, re-keyed") {
		t.Fatalf("only the fresh entry goes to the model: %v", model.prompts)
	}
}

func TestSetupWithNothingToFindRunsOnceDB(t *testing.T) {
	_, q, ws := testDB(t)
	ctx := context.Background()
	brand(t, q, ws) // no competitor names
	svc := &Service{Q: q, Fetch: (&site{pages: map[string]string{}}).fetch, Model: &fakeModel{}, Logger: zerolog.Nop()}
	created, unresolved, err := svc.Setup(ctx, ws)
	if err != nil || created != 0 || len(unresolved) != 0 {
		t.Fatalf("got %d %v %v", created, unresolved, err)
	}
	if _, err := q.GetCompetitorSetup(ctx, ws); err != nil {
		t.Fatalf("setup must be recorded even when empty: %v", err)
	}
}

func TestSetupStopsAtTheLimitDB(t *testing.T) {
	_, q, ws := testDB(t)
	ctx := context.Background()
	brand(t, q, ws, "Aaa", "Bbb", "Ccc", "Ddd")
	svc := &Service{
		Q: q, Fetch: (&site{pages: map[string]string{}}).fetch, Model: &fakeModel{}, Logger: zerolog.Nop(),
		Limit: func(context.Context, string) int { return FreeLimit },
		Search: func(_ context.Context, name string) ([]monitor.ExaWebResult, error) {
			return []monitor.ExaWebResult{{Title: name, URL: "https://" + strings.ToLower(name) + ".com"}}, nil
		},
	}
	created, _, err := svc.Setup(ctx, ws)
	if err != nil || created != FreeLimit {
		t.Fatalf("created %d, want %d (%v)", created, FreeLimit, err)
	}
	if _, err := svc.Add(ctx, ws, "Eee", "https://eee.com"); !errors.Is(err, ErrLimit) {
		t.Fatalf("adding past the limit: %v", err)
	}
}
