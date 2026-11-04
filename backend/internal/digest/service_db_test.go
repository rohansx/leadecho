package digest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

type fakeSummarizer struct {
	text string
	err  error
}

func (f fakeSummarizer) SummarizeDigest(context.Context, string, string) (string, error) {
	return f.text, f.err
}

type recorder struct {
	mu    sync.Mutex
	calls []map[string]any
}

func (r *recorder) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(req.Body).Decode(&body)
		r.mu.Lock()
		r.calls = append(r.calls, body)
		r.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (r *recorder) matching(pred func(map[string]any) bool) []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []map[string]any
	for _, c := range r.calls {
		if pred(c) {
			out = append(out, c)
		}
	}
	return out
}

func setupWorkspace(t *testing.T, ctx context.Context, pool *pgxpool.Pool, q *database.Queries, tag string) (wsID, email string) {
	t.Helper()
	n := time.Now().UnixNano()
	ws, err := q.CreateWorkspace(ctx, database.CreateWorkspaceParams{ClerkOrgID: fmt.Sprintf("org_%s_%d", tag, n), Name: "Digest " + tag, Slug: fmt.Sprintf("digest-%s-%d", tag, n)})
	if err != nil {
		t.Fatal(err)
	}
	email = fmt.Sprintf("owner-%s-%d@leadecho.test", tag, n)
	if _, err := q.CreateUser(ctx, database.CreateUserParams{
		ClerkUserID: fmt.Sprintf("u_%s_%d", tag, n), WorkspaceID: ws.ID, Email: email, Name: "Owner", Role: database.UserRoleAdmin,
	}); err != nil {
		t.Fatal(err)
	}
	return ws.ID, email
}

func exec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
}

func seedPeriod(t *testing.T, ctx context.Context, pool *pgxpool.Pool, wsID string, at time.Time) {
	t.Helper()
	var kwID string
	if err := pool.QueryRow(ctx, `WITH p AS (INSERT INTO monitoring_profiles (workspace_id, name) VALUES ($1, 'p') RETURNING id)
		INSERT INTO keywords (workspace_id, term, profile_id) SELECT $1, 'noise', id FROM p RETURNING id`, wsID).Scan(&kwID); err != nil {
		t.Fatal(err)
	}
	// 12 low-fit mentions on one keyword (noisy) + 2 high-fit buyers.
	for i := 0; i < 12; i++ {
		exec(t, ctx, pool, `INSERT INTO mentions (workspace_id, keyword_id, platform, platform_id, url, content, relevance_score, intent, created_at)
			VALUES ($1, $2, 'reddit', $3, $4, 'meh', 1.5, 'general', $5)`, wsID, kwID, fmt.Sprintf("n%d-%s", i, wsID), fmt.Sprintf("https://reddit.com/n/%d", i), at)
	}
	var mentionID string
	if err := pool.QueryRow(ctx, `INSERT INTO mentions (workspace_id, platform, platform_id, url, title, content, relevance_score, intent, created_at)
		VALUES ($1, 'hackernews', $2, 'https://news.ycombinator.com/item?id=1', 'Need a tool for X', 'buying', 9.1, 'buy_signal', $3) RETURNING id`,
		wsID, "b1-"+wsID, at).Scan(&mentionID); err != nil {
		t.Fatal(err)
	}
	exec(t, ctx, pool, `INSERT INTO mentions (workspace_id, platform, platform_id, url, title, content, relevance_score, intent, created_at)
		VALUES ($1, 'reddit', $2, 'https://reddit.com/b/2', 'Alternatives to Y?', 'switching', 8.0, 'recommendation_ask', $3)`, wsID, "b2-"+wsID, at)
	// Outside the period: must not count.
	exec(t, ctx, pool, `INSERT INTO mentions (workspace_id, platform, platform_id, url, content, relevance_score, intent, created_at)
		VALUES ($1, 'reddit', $2, 'https://reddit.com/old', 'old', 9.9, 'buy_signal', $3)`, wsID, "old-"+wsID, at.Add(-72*time.Hour))
	exec(t, ctx, pool, `INSERT INTO replies (mention_id, workspace_id, content, status, posted_at) VALUES ($1, $2, 'hi', 'posted', $3)`, mentionID, wsID, at)
	var linkID string
	if err := pool.QueryRow(ctx, `INSERT INTO utm_links (workspace_id, code, destination_url, utm_source, utm_medium) VALUES ($1, $2, 'https://x.io', 'hn', 'reply') RETURNING id`,
		wsID, fmt.Sprintf("c%d", time.Now().UnixNano())).Scan(&linkID); err != nil {
		t.Fatal(err)
	}
	exec(t, ctx, pool, `INSERT INTO utm_events (utm_link_id, event_type, created_at) VALUES ($1, 'click', $2), ($1, 'click', $2), ($1, 'purchase', $2)`, linkID, at)
	exec(t, ctx, pool, `UPDATE utm_events SET revenue_cents = 4900 WHERE utm_link_id = $1 AND event_type = 'purchase'`, linkID)
}

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated database.
func TestDigestServiceDB(t *testing.T) {
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

	// Wednesday 2026-11-04 09:31 UTC: the daily digest (09:30) is due, weekly (monday) is not.
	now := time.Date(2026, 11, 4, 9, 31, 0, 0, time.UTC)
	wsID, owner := setupWorkspace(t, ctx, pool, q, "a")
	otherWS, _ := setupWorkspace(t, ctx, pool, q, "b")
	seedPeriod(t, ctx, pool, wsID, now.Add(-2*time.Hour))
	exec(t, ctx, pool, `INSERT INTO mentions (workspace_id, platform, platform_id, url, content, relevance_score, intent, created_at)
		VALUES ($1, 'reddit', $2, 'https://reddit.com/other', 'other tenant', 9.9, 'buy_signal', $3)`, otherWS, "o-"+otherWS, now.Add(-time.Hour))

	mail, slack := &recorder{}, &recorder{}
	mailSrv, slackSrv := mail.server(t), slack.server(t)
	svc := NewService(q, fakeSummarizer{text: "AI SUMMARY"}, "re_test", "LeadEcho <d@x.io>", "https://app.test", zerolog.Nop())
	svc.mail.Endpoint = mailSrv.URL
	svc.now = func() time.Time { return now }
	svc.slackClient = slackSrv.Client()
	svc.slackOK = func(string) bool { return true }

	t.Run("defaults resolve owner email", func(t *testing.T) {
		s, err := svc.Settings(ctx, wsID)
		if err != nil || !s.DailyEnabled || s.DailyTime != "09:30" || len(s.Recipients) != 1 || s.Recipients[0] != owner {
			t.Fatalf("settings %+v err %v", s, err)
		}
	})

	t.Run("build is period- and tenant-scoped", func(t *testing.T) {
		p, _ := DefaultSchedule().Due(KindDaily, now)
		d, err := svc.Build(ctx, wsID, KindDaily, p)
		if err != nil {
			t.Fatal(err)
		}
		if d.TotalMentions() != 14 || d.Buyers() != 2 || d.RepliesPosted != 1 || d.Clicks != 2 || d.RevenueCents != 4900 {
			t.Fatalf("mentions=%d buyers=%d replies=%d clicks=%d rev=%d", d.TotalMentions(), d.Buyers(), d.RepliesPosted, d.Clicks, d.RevenueCents)
		}
		if len(d.NoisyKeywords) != 1 || d.NoisyKeywords[0].Term != "noise" {
			t.Fatalf("noisy %+v", d.NoisyKeywords)
		}
		if len(d.TopLeads) == 0 || d.TopLeads[0].Title != "Need a tool for X" {
			t.Fatalf("top leads %+v", d.TopLeads)
		}
	})

	t.Run("invalid settings rejected", func(t *testing.T) {
		if _, err := svc.SaveSettings(ctx, wsID, Schedule{DailyTime: "nope", WeeklyDay: "monday"}); err == nil {
			t.Fatal("want validation error")
		}
	})

	sched := DefaultSchedule()
	sched.Recipients = []string{"team@leadecho.test", owner}
	sched.Channels = []string{"email", "slack"}
	if _, err := svc.SaveSettings(ctx, wsID, sched); err != nil {
		t.Fatal(err)
	}
	exec(t, ctx, pool, `UPDATE workspaces SET settings = jsonb_set(COALESCE(settings, '{}'), '{webhooks}', $2::jsonb) WHERE id = $1`,
		wsID, fmt.Sprintf(`{"slack_url":%q}`, slackSrv.URL))
	// Tenant b opts out of every channel: never sent.
	if _, err := svc.SaveSettings(ctx, otherWS, Schedule{DailyTime: "09:30", WeeklyDay: "monday", DailyEnabled: true}); err != nil {
		t.Fatal(err)
	}

	toOwner := func(c map[string]any) bool {
		to, _ := c["to"].([]any)
		return len(to) > 0 && to[len(to)-1] == owner
	}
	slackForWS := func(c map[string]any) bool {
		s, _ := c["text"].(string)
		return strings.Contains(s, "Need a tool for X")
	}

	t.Run("due digest is sent once on the chosen channels", func(t *testing.T) {
		svc.RunDue(ctx)
		svc.RunDue(ctx) // second tick: idempotent
		emails := mail.matching(toOwner)
		if len(emails) != 1 {
			t.Fatalf("emails to owner = %d, want 1", len(emails))
		}
		if subj, _ := emails[0]["subject"].(string); !strings.HasPrefix(subj, "LeadEcho daily digest: 2 buyers") {
			t.Fatalf("subject %q", subj)
		}
		if n := len(slack.matching(slackForWS)); n != 1 {
			t.Fatalf("slack posts = %d, want 1", n)
		}
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM digest_runs WHERE workspace_id = $1 AND kind = 'daily'`, wsID).Scan(&status); err != nil || status != "sent" {
			t.Fatalf("run status %q err %v", status, err)
		}
		var otherRuns int
		_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM digest_runs WHERE workspace_id = $1`, otherWS).Scan(&otherRuns)
		if otherRuns != 0 {
			t.Fatalf("opted-out workspace has %d runs", otherRuns)
		}
	})

	t.Run("slack only when chosen", func(t *testing.T) {
		before := len(slack.matching(func(map[string]any) bool { return true }))
		emailOnly := DefaultSchedule()
		d := Data{Kind: KindDaily, Platforms: []Count{{"reddit", 1}}}
		if status, _ := svc.deliver(ctx, wsID, emailOnly, d); status != "sent" {
			t.Fatalf("status %q", status)
		}
		if after := len(slack.matching(func(map[string]any) bool { return true })); after != before {
			t.Fatal("slack must not be used when only email is chosen")
		}
	})

	t.Run("preview uses llm summary for weekly, template fallback on error", func(t *testing.T) {
		html, err := svc.Preview(ctx, wsID, KindWeekly)
		if err != nil || !strings.Contains(html, "AI SUMMARY") {
			t.Fatalf("weekly preview err=%v", err)
		}
		svc.llm = fakeSummarizer{err: errors.New("no AI provider configured")}
		html, _ = svc.Preview(ctx, wsID, KindWeekly)
		if !strings.Contains(html, "3 buyers, 0 competitor complaints, 1 noisy keyword, $49 attributed.") {
			t.Fatal("template fallback missing")
		}
		html, _ = svc.Preview(ctx, wsID, KindDaily)
		if strings.Contains(html, "competitor complaints") {
			t.Fatal("daily digest has no summary paragraph")
		}
	})

	claim := func(p Period) (string, error) {
		return q.ClaimDigestRun(ctx, database.ClaimDigestRunParams{WorkspaceID: wsID, Kind: "weekly", PeriodStart: p.Start, PeriodEnd: p.End})
	}
	age := func(id string, d time.Duration) {
		if _, err := pool.Exec(ctx, `UPDATE digest_runs SET updated_at = now() - $2::interval WHERE id = $1`, id, fmt.Sprintf("%d seconds", int(d.Seconds()))); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("failed run is retried after backoff", func(t *testing.T) {
		p := Period{Start: now.Add(-48 * time.Hour), End: now.Add(-24 * time.Hour)}
		id, err := claim(p)
		if err != nil {
			t.Fatal(err)
		}
		_ = q.FinishDigestRun(ctx, database.FinishDigestRunParams{ID: id, WorkspaceID: wsID, Status: "failed", Error: "x"})
		if _, err := claim(p); err == nil {
			t.Fatal("failed run must wait for backoff before retry")
		}
		age(id, 11*time.Minute)
		if _, err := claim(p); err != nil {
			t.Fatalf("failed run should be reclaimable after backoff: %v", err)
		}
		if _, err := claim(p); err == nil {
			t.Fatal("fresh pending run must not be reclaimable")
		}
	})

	t.Run("stale pending run (crashed sender) is reclaimed", func(t *testing.T) {
		p := Period{Start: now.Add(-72 * time.Hour), End: now.Add(-48 * time.Hour)}
		id, err := claim(p)
		if err != nil {
			t.Fatal(err)
		}
		age(id, 20*time.Minute)
		if _, err := claim(p); err != nil {
			t.Fatalf("stale pending run should be reclaimable: %v", err)
		}
	})
}
