package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/ai"
	"leadecho/internal/database"
)

func companies(jobs []jobPost) []string {
	out := []string{}
	for _, j := range jobs {
		out = append(out, j.Company)
	}
	return out
}

func TestCarryOverRoundTrip(t *testing.T) {
	now := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	round := &fundingRound{Type: "seed", Amount: "$4 million", Date: "2026-11-13", URL: "https://news.example/r", domain: "drillbit.example"}
	unreached := []jobPost{
		{Company: "Acme", Title: "Engineer", Location: "NYC", URL: "https://news.ycombinator.com/item?id=1", CreatedUTC: now.Unix(), Source: sourceHN, CompanyURL: "https://acme.example"},
		{Company: "Drillbit", URL: "https://news.example/r", Source: sourceFunding, Funding: round, SizeFits: true, Country: "US"},
	}
	earlier := now.Add(-3 * 24 * time.Hour)
	carry, dropped := carryOver(unreached, map[string]time.Time{"acme": earlier}, now)
	if dropped != 0 || len(carry) != 2 || !carry[0].Seen.Equal(earlier) || !carry[1].Seen.Equal(now) {
		t.Fatalf("carry: %+v dropped=%d", carry, dropped)
	}
	// Through the settings JSON and back: the same entries, the round's website included.
	var stored []carriedJob
	if err := json.Unmarshal(jsonBytes(carry), &stored); err != nil {
		t.Fatal(err)
	}
	rep := &hiringReport{}
	jobs, seen := carriedJobs(stored, now, func(jobPost) string { return "" }, rep)
	if !reflect.DeepEqual(jobs, unreached) || !seen["acme"].Equal(earlier) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", jobs, unreached)
	}

	// Ten days after it was first seen an entry is dropped, and so is one the
	// scan would skip anyway (stored since, on the skip list).
	stored[0].Seen = now.Add(-hiringCarryAge - time.Hour)
	jobs, _ = carriedJobs(stored, now, func(j jobPost) string {
		if j.Company == "Drillbit" {
			return "already_stored_this_month"
		}
		return ""
	}, rep)
	if len(jobs) != 0 || rep.Skipped["carry_expired"] != 1 || rep.Skipped["already_stored_this_month"] != 1 {
		t.Fatalf("expired/skipped: %v %v", companies(jobs), rep.Skipped)
	}

	// Bounded: the oldest are kept, the rest counted.
	var many []jobPost
	for i := 0; i < maxHiringCarried+5; i++ {
		many = append(many, jobPost{Company: fmt.Sprintf("Co %d", i), Title: "Engineer", URL: "https://x.example"})
	}
	if carry, dropped := carryOver(many, nil, now); len(carry) != maxHiringCarried || dropped != 5 || carry[0].Company != "Co 0" {
		t.Fatalf("bound: %d kept, %d dropped", len(carry), dropped)
	}
}

func TestLimitFunded(t *testing.T) {
	f := &fundingRound{Type: "seed", Date: "2026-11-13"}
	funded := func(name string) jobPost { return jobPost{Company: name, Funding: f} }
	jobs := []jobPost{funded("F1"), funded("F2"), funded("F3"), {Company: "P1", Title: "Engineer"}, {Company: "P2", Title: "Engineer", Funding: f}}
	// Starter: 3 people searches a day, 1 of them for a funded company. A
	// posting that also carries a round is a posting.
	if got := fundedShare(starterHiringCompanies); got != 1 {
		t.Fatalf("starter share = %d, want 1", got)
	}
	if got := fundedShare(maxHiringCompaniesPerRun); got != 6 {
		t.Fatalf("pro share = %d, want 6", got)
	}
	queue, over := limitFunded(jobs, 1)
	if got, want := companies(queue), []string{"F1", "P1", "P2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("queue = %v, want %v", got, want)
	}
	if got, want := companies(over), []string{"F2", "F3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("over = %v, want %v", got, want)
	}
	if queue, over := limitFunded(jobs, 6); len(queue) != 5 || len(over) != 0 {
		t.Errorf("within the share nothing moves: %v %v", companies(queue), companies(over))
	}
}

func TestSignalPlatform(t *testing.T) {
	for _, c := range []struct {
		job  jobPost
		want database.PlatformType
	}{
		{jobPost{Source: sourceHN, URL: "https://news.ycombinator.com/item?id=1"}, database.PlatformTypeHackernews},
		{jobPost{Source: sourceHN, URL: "https://acme.example/jobs"}, database.PlatformTypeLinkedin},
		{jobPost{Source: sourceLinkedIn, URL: "https://www.linkedin.com/jobs/view/1"}, database.PlatformTypeLinkedin},
		{jobPost{Source: sourceFunding, URL: "https://news.ycombinator.com/item?id=2"}, database.PlatformTypeLinkedin},
	} {
		if got := signalPlatform(c.job); got != string(c.want) {
			t.Errorf("signalPlatform(%s %s) = %s, want %s", c.job.Source, c.job.URL, got, c.want)
		}
	}
}

// screenRecorder notes which companies each screen call was given.
type screenRecorder struct {
	fakeHiringPlanner
	screened *[][]string
}

func (p screenRecorder) ScreenHiringJobs(ctx context.Context, ws string, in ai.HiringScreen) ([]ai.ProspectFit, error) {
	var names []string
	for _, j := range in.Jobs {
		names = append(names, j.Company)
	}
	*p.screened = append(*p.screened, names)
	return p.fakeHiringPlanner.ScreenHiringJobs(ctx, ws, in)
}

// DB-backed: five companies pass the screen on a plan with three people
// searches a day. The two not reached are stored in the workspace's hiring
// state and tried first by the next run, without being screened again; a
// six-person company is dropped by the plan's minimum size.
func TestScanHiringCarriesUnreachedDB(t *testing.T) {
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
	var ws string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug, settings) VALUES ('hc-'||gen_random_uuid(), 'hc', 'hc-'||gen_random_uuid(),
		'{"hiring_signals":{"last_run":"2026-01-01T00:00:00Z","funding_limit":0,"plan":{"applies":true,"roles":["software engineer"],"buyer_titles":"CTO","max_company_size":5000,"why":"they screen engineers"}}}')
		RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)
	if _, err := pool.Exec(ctx, `INSERT INTO brand_profiles (workspace_id, product, overview, target_audience) VALUES ($1, 'Assessments', 'Technical skill assessments', 'Engineering leaders')`, ws); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	// Newest first. The postings differ in more than the name: near-identical
	// texts are dropped on insert as duplicates.
	names := []string{"Northwind Robotics", "Bluebird Health Analytics", "Tiny Co", "Granite Payments Platform", "Harbor Logistics Cloud", "Quartz Education Systems"}
	roles := []string{"Software Engineer", "Senior Backend Software Engineer", "Software Engineer", "Staff Software Engineer, Payments", "Infrastructure Software Engineer", "Principal Software Engineer"}
	places := []string{"New York", "Berlin, Germany", "Austin", "Toronto, Canada", "Lisbon, Portugal", "Sydney, Australia"}
	var rows []string
	for i, n := range names {
		rows = append(rows, fmt.Sprintf(`{"company":%q,"posted":%q,"text":"%s | %s | %s | https://%s.test/jobs","url":"https://news.ycombinator.com/item?id=%d"}`,
			n, now.Add(-time.Duration(i+1)*time.Minute).Format(time.RFC3339), n, roles[i], places[i], companyKey(n), i+1))
	}
	client, calls := fakeTreg(t, func(tool string, body map[string]any) (int, string) {
		switch tool {
		case tregHNHiringTool:
			return 200, `{"rows":[` + strings.Join(rows, ",") + `]}`
		case tregBlitzJobsTool:
			return 200, `{"results":[],"cursor":null}`
		case tregJobsTool:
			return 200, `{"output":{"data":{"items":[]}}}`
		case tregCompanyTool:
			name, _ := body["name"].(string)
			size := 50
			if name == "Tiny Co" {
				size = 6
			}
			return 200, fmt.Sprintf(`{"output":{"name":%q,"domain":"%s.test","employees":%d}}`, name, companyKey(name), size)
		case tregPeopleTool:
			return 200, `{"output":{"people":[]}}` // nobody found: the search is still paid for
		}
		return 400, `{"error":"unexpected tool"}`
	})
	var screened [][]string
	m := &Monitor{q: q, logger: zerolog.Nop(), treg: client, hiringPlanner: screenRecorder{screened: &screened}}
	state := func() hiringState {
		t.Helper()
		raw, err := q.GetWorkspaceSettingsKey(ctx, ws, hiringSettingsKey)
		if err != nil {
			t.Fatal(err)
		}
		var st hiringState
		if err := json.Unmarshal(raw, &st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	searchedFor := func() []string {
		var out []string
		for _, c := range *calls {
			if c.tool == tregPeopleTool {
				q, _ := c.body["q"].(string)
				_, company, _ := strings.Cut(q, " at ")
				out = append(out, company)
			}
		}
		return out
	}

	m.scanHiring(ctx, ws)
	st := state()
	var waiting []string
	for _, c := range st.Carry {
		waiting = append(waiting, c.Company)
	}
	if !reflect.DeepEqual(waiting, names[4:]) || st.LastReport.CarriedOver != 2 || st.LastReport.Skipped["too_small"] != 1 {
		t.Fatalf("first run: carry=%v report=%s", waiting, jsonBytes(st.LastReport))
	}
	if got := searchedFor(); !reflect.DeepEqual(got, []string{names[0], names[1], names[3]}) {
		t.Fatalf("first run searched for %v", got)
	}
	if len(screened) != 1 || len(screened[0]) != len(names) {
		t.Fatalf("first run screened %v", screened)
	}

	// The next run: the thread's posts are older than the last scan, so only
	// the carried companies are left, and they are not screened again.
	*calls, screened = nil, nil
	m.scanHiring(withForcedScan(ctx, ScanHiring), ws)
	st = state()
	if got := searchedFor(); !reflect.DeepEqual(got, names[4:]) {
		t.Fatalf("second run searched for %v", got)
	}
	for _, batch := range screened {
		if len(batch) != 0 {
			t.Errorf("second run screened %v again", batch)
		}
	}
	if len(st.Carry) != 0 || st.LastReport.CarriedIn != 2 || st.LastReport.CarriedOver != 0 {
		t.Errorf("second run: carry=%+v report=%s", st.Carry, jsonBytes(st.LastReport))
	}
	// One signal per company, stored under the platform its link leads to.
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM mentions WHERE workspace_id = $1 AND platform = 'hackernews' AND url LIKE 'https://news.ycombinator.com/%'`, ws).Scan(&n); err != nil || n != 5 {
		t.Errorf("stored hackernews signals = %d (err %v), want 5", n, err)
	}

	// A third run finds every company stored already, whatever its platform.
	*calls = nil
	if _, err := pool.Exec(ctx, `UPDATE workspaces SET settings = jsonb_set(settings, '{hiring_signals,last_scan}', '"2026-01-01T00:00:00Z"') WHERE id = $1`, ws); err != nil {
		t.Fatal(err)
	}
	m.scanHiring(withForcedScan(ctx, ScanHiring), ws)
	if got := searchedFor(); len(got) != 0 || state().LastReport.Skipped["already_stored_this_month"] != 5 {
		t.Errorf("third run: searched for %v, report %s", got, jsonBytes(state().LastReport))
	}
}
