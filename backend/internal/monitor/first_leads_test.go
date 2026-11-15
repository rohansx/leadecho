package monitor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

func TestFirstLeadsOwed(t *testing.T) {
	now := scanT0
	for name, tc := range map[string]struct {
		settings            string
		created, oldestLead time.Time
		want                bool
	}{
		"finished onboarding since the email exists":         {`{"onboarding":{"completed":true,"completed_at":"2026-11-12T08:00:00Z"}}`, now.Add(-time.Hour), time.Time{}, true},
		"the same, an old workspace that only now onboarded": {`{"onboarding":{"completed":true,"completed_at":"2026-11-12T08:00:00Z"}}`, now.Add(-90 * 24 * time.Hour), now.Add(-time.Minute), true},
		"already sent":                                         {`{"onboarding":{"completed_at":"2026-11-12T08:00:00Z"},"first_leads_email":{"sent_at":"2026-11-12T08:30:00Z"}}`, now.Add(-time.Hour), now, false},
		"ruled out before":                                     {`{"first_leads_email":{"skipped":true}}`, now.Add(-time.Hour), now, false},
		"existing customer: old workspace":                     {`{"onboarding":{"completed":true}}`, now.Add(-30 * 24 * time.Hour), now.Add(-time.Minute), false},
		"old workspace without a lead yet":                     {`{"onboarding":{"completed":true}}`, now.Add(-8 * 24 * time.Hour), time.Time{}, false},
		"onboarded days before the email, no lead yet":         {`{"onboarding":{"completed":true}}`, now.Add(-3 * 24 * time.Hour), time.Time{}, true},
		"onboarded days before the email, first lead just now": {`{"onboarding":{"completed":true}}`, now.Add(-3 * 24 * time.Hour), now.Add(-10 * time.Minute), true},
		"onboarded days before the email, has had leads since": {`{"onboarding":{"completed":true}}`, now.Add(-3 * 24 * time.Hour), now.Add(-2 * 24 * time.Hour), false},
		"unreadable settings":                                  {`not json`, now.Add(-time.Hour), time.Time{}, false},
	} {
		if got := firstLeadsOwed([]byte(tc.settings), tc.created, tc.oldestLead, now); got != tc.want {
			t.Errorf("%s: owed = %v, want %v", name, got, tc.want)
		}
	}
}

func TestFirstLeadsEmailSaysHowManyAndNamesTheBest(t *testing.T) {
	text := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
	top := []database.Mention{
		{Platform: "linkedin", Title: text("Acme is hiring: Head of Sales"), Content: "Who to contact: Ann Lee, CRO at Acme. ann@acme.test", AuthorUsername: text("Ann Lee")},
		{Platform: "hackernews", Title: text(`Ask HN: a CRM for a <small> agency?`), Content: "we need one"},
	}
	subject, body := firstLeadsEmail(5, top, "https://app.leadecho.test")
	if subject != "LeadEcho: your first 5 leads are ready" {
		t.Errorf("subject = %q", subject)
	}
	for _, want := range []string{"Your first 5 leads are ready", "Acme is hiring: Head of Sales", "a CRM for a &lt;small&gt; agency?", "and 3 more.", `href="https://app.leadecho.test/inbox"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "@") || strings.Contains(body, "Ann Lee") {
		t.Errorf("the email names the contact:\n%s", body)
	}
	subject, body = firstLeadsEmail(1, top[1:], "https://app.leadecho.test")
	if subject != "LeadEcho: your first lead is ready" || strings.Contains(body, "more.") {
		t.Errorf("one lead: %q\n%s", subject, body)
	}
}

// The scan asked for when onboarding ends is the scheduled scan started
// early: the daily company scan keeps its once-a-day claim. An operator's
// "scan now" skips it.
func TestOnboardingScanKeepsTheDailyClaim(t *testing.T) {
	onb, _ := NewManualScan(ScanPosts, ScanByOnboarding, scanT0)
	if ctx := scanContext(onb); forcedScan(ctx) != "" || hiringClaimInterval(ctx) != hiringInterval {
		t.Fatalf("onboarding scan is forced: %q", forcedScan(ctx))
	}
	op, _ := NewManualScan(ScanHiring, "ops@example.com", scanT0)
	if ctx := scanContext(op); forcedScan(ctx) != ScanHiring || hiringClaimInterval(ctx) != 0 {
		t.Fatalf("operator scan is not forced: %q", forcedScan(ctx))
	}
}

func TestCompanyScanProgress(t *testing.T) {
	now := scanT0
	stamp := func(t time.Time) string { return t.Format(time.RFC3339) }
	for name, tc := range map[string]struct {
		raw   string
		state string
		next  time.Time
	}{
		"never ran":            {``, CompanyScanNever, time.Time{}},
		"only countries set":   {`{"countries":["US"]}`, CompanyScanNever, time.Time{}},
		"claimed, no report":   {fmt.Sprintf(`{"last_run":%q}`, stamp(now.Add(-5*time.Minute))), CompanyScanRunning, time.Time{}},
		"yesterday's report":   {fmt.Sprintf(`{"last_run":%q,"last_report":{"at":%q}}`, stamp(now.Add(-5*time.Minute)), stamp(now.Add(-24*time.Hour))), CompanyScanRunning, time.Time{}},
		"finished":             {fmt.Sprintf(`{"last_run":%q,"last_report":{"at":%q}}`, stamp(now.Add(-2*time.Hour)), stamp(now.Add(-2*time.Hour))), CompanyScanDone, now.Add(22 * time.Hour)},
		"ended without report": {fmt.Sprintf(`{"last_run":%q}`, stamp(now.Add(-2*time.Hour))), CompanyScanDone, now.Add(22 * time.Hour)},
		// Every job search failed: the claim was moved so the next run is due in an hour.
		"retry in an hour":    {fmt.Sprintf(`{"last_run":%q,"last_report":{"at":%q,"stopped":"every job search failed"}}`, stamp(now.Add(-23*time.Hour)), stamp(now.Add(-time.Minute))), CompanyScanDone, now.Add(time.Hour)},
		"product with no fit": {fmt.Sprintf(`{"last_run":%q,"plan":{"applies":false}}`, stamp(now.Add(-time.Minute))), CompanyScanNoFit, time.Time{}},
	} {
		state, next := CompanyScanProgress([]byte(tc.raw), now)
		if state != tc.state || !next.Equal(tc.next) {
			t.Errorf("%s: %s %v, want %s %v", name, state, next, tc.state, tc.next)
		}
	}
}

type fakeMailer struct {
	mu   sync.Mutex
	sent []string // "to|subject|html"
}

func (f *fakeMailer) Enabled() bool { return true }
func (f *fakeMailer) Send(_ context.Context, to []string, subject, html string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, strings.Join(to, ",")+"|"+subject+"|"+html)
	return nil
}
func (f *fakeMailer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

// DB-backed: the first-leads email goes out once per workspace, however many
// scans and monitor processes notice the leads, and never to a workspace that
// was already using LeadEcho.
func TestFirstLeadsEmailDB(t *testing.T) {
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

	// workspace makes a workspace with an owner; age is how long ago it was created.
	workspace := func(settings string, age time.Duration) (ws, owner string) {
		t.Helper()
		if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug, settings, created_at) VALUES ('fl-'||gen_random_uuid(), 'fl', 'fl-'||gen_random_uuid(), $1::jsonb, now() - make_interval(secs => $2)) RETURNING id`,
			settings, age.Seconds()).Scan(&ws); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws) })
		owner = "owner-" + ws + "@leadecho.test"
		if _, err := pool.Exec(ctx, `INSERT INTO users (clerk_user_id, workspace_id, email, name, role) VALUES ($1, $2, $3, 'Owner', 'admin')`, "fl-"+ws, ws, owner); err != nil {
			t.Fatal(err)
		}
		return ws, owner
	}
	// lead stores a mention that is a lead by the Leads-tab rule; signal "" = a post.
	lead := func(ws, title, signal string, age time.Duration) {
		t.Helper()
		meta := `{}`
		if signal != "" {
			meta = fmt.Sprintf(`{"signal":%q,"confirmed":true}`, signal)
		}
		if _, err := pool.Exec(ctx, `
INSERT INTO mentions (workspace_id, platform, platform_id, url, title, content, status, intent, relevance_score, scoring_metadata, platform_metadata, engagement_metrics, keyword_matches, created_at)
VALUES ($1, 'hackernews', gen_random_uuid()::text, 'https://example.test/p', $2, 'Who to contact: Ann Lee, ann@acme.test', 'new', 'buy_signal', 8, '{"stage":"stage3_classified"}', $3::jsonb, '{}', '{}', now() - make_interval(secs => $4))`,
			ws, title, meta, age.Seconds()); err != nil {
			t.Fatal(err)
		}
	}
	flag := func(ws string) string {
		t.Helper()
		raw, err := q.GetWorkspaceSettingsKey(ctx, ws, firstLeadsEmailKey)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	monitors := func() (*fakeMailer, []*Monitor) { // two processes, one outbox
		mail := &fakeMailer{}
		var ms []*Monitor
		for range 2 {
			m := &Monitor{q: q, logger: zerolog.Nop()}
			m.SetMail(mail, "https://app.leadecho.test/")
			ms = append(ms, m)
		}
		return mail, ms
	}
	const onboarded = `{"onboarding":{"completed":true,"step":3,"completed_at":"2026-11-15T08:00:00Z"}}`

	t.Run("sent once, whoever notices", func(t *testing.T) {
		ws, owner := workspace(onboarded, time.Hour)
		mail, ms := monitors()
		ms[0].notifyFirstLeads(ctx, ws)
		if mail.count() != 0 || flag(ws) != "" {
			t.Fatalf("no lead yet: %d sent, flag %q", mail.count(), flag(ws))
		}
		lead(ws, "Ask HN: a CRM for a small agency?", "", time.Minute)
		lead(ws, "Acme is hiring: Head of Sales", "hiring", time.Minute)
		lead(ws, "Looking for a CRM", "", 2*time.Minute)
		var wg sync.WaitGroup
		for i := range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ms[i%2].notifyFirstLeads(ctx, ws)
			}()
		}
		wg.Wait()
		ms[1].notifyFirstLeads(ctx, ws) // and a later scan
		if mail.count() != 1 {
			t.Fatalf("sent %d emails, want 1: %v", mail.count(), mail.sent)
		}
		got := mail.sent[0]
		if !strings.HasPrefix(got, owner+"|LeadEcho: your first 3 leads are ready|") || !strings.Contains(got, "and 1 more.") ||
			!strings.Contains(got, `href="https://app.leadecho.test/inbox"`) || strings.Contains(got, "ann@acme.test") {
			t.Fatalf("email = %s", got)
		}
		if f := flag(ws); !strings.Contains(f, `"sent_at"`) || !strings.Contains(f, `"leads": 3`) {
			t.Fatalf("flag = %s", f)
		}
		var logged int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE workspace_id = $1 AND recipient = $2 AND sent_at IS NOT NULL`, ws, owner).Scan(&logged)
		if logged != 1 {
			t.Fatalf("notifications logged = %d", logged)
		}
	})

	t.Run("goes to the alert address when one is set", func(t *testing.T) {
		ws, _ := workspace(`{"onboarding":{"completed_at":"2026-11-15T08:00:00Z"},"webhooks":{"email_to":"sales@acme.test"}}`, time.Hour)
		lead(ws, "Looking for a CRM", "", time.Minute)
		mail, ms := monitors()
		ms[0].notifyFirstLeads(ctx, ws)
		if mail.count() != 1 || !strings.HasPrefix(mail.sent[0], "sales@acme.test|LeadEcho: your first lead is ready|") {
			t.Fatalf("sent = %v", mail.sent)
		}
	})

	for name, tc := range map[string]struct {
		settings        string
		age, oldestLead time.Duration
	}{
		"a customer from before the email":                    {`{"onboarding":{"completed":true,"step":3}}`, 30 * 24 * time.Hour, 20 * 24 * time.Hour},
		"an old workspace whose first lead arrives only now":  {`{"onboarding":{"completed":true,"step":3}}`, 30 * 24 * time.Hour, time.Minute},
		"a days-old workspace that has had leads for a while": {`{"onboarding":{"completed":true,"step":3}}`, 3 * 24 * time.Hour, 2 * 24 * time.Hour},
	} {
		t.Run("not sent to "+name, func(t *testing.T) {
			ws, _ := workspace(tc.settings, tc.age)
			lead(ws, "Looking for a CRM", "", tc.oldestLead)
			lead(ws, "Another one", "", time.Minute)
			mail, ms := monitors()
			ms[0].notifyFirstLeads(ctx, ws)
			ms[1].notifyFirstLeads(ctx, ws)
			if mail.count() != 0 {
				t.Fatalf("sent = %v", mail.sent)
			}
			if f := flag(ws); !strings.Contains(f, `"skipped"`) {
				t.Fatalf("not ruled out for good: flag = %q", f)
			}
		})
	}

	t.Run("sent to a days-old workspace whose first lead arrives now", func(t *testing.T) {
		ws, owner := workspace(`{"onboarding":{"completed":true,"step":3}}`, 3*24*time.Hour)
		lead(ws, "Looking for a CRM", "", time.Minute)
		mail, ms := monitors()
		ms[0].notifyFirstLeads(ctx, ws)
		ms[0].notifyFirstLeads(ctx, ws)
		if mail.count() != 1 || !strings.HasPrefix(mail.sent[0], owner+"|") {
			t.Fatalf("sent = %v", mail.sent)
		}
	})

	t.Run("nothing without a mail provider", func(t *testing.T) {
		ws, _ := workspace(onboarded, time.Hour)
		lead(ws, "Looking for a CRM", "", time.Minute)
		(&Monitor{q: q, logger: zerolog.Nop()}).notifyFirstLeads(ctx, ws)
		if f := flag(ws); f != "" {
			t.Fatalf("flag set without sending: %s", f)
		}
	})
}
