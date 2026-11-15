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
	"leadecho/internal/enrich"
)

type fakeHiringPlanner struct{}

func (fakeHiringPlanner) PlanHiringSignals(context.Context, string, string, string, string) (*ai.HiringPlan, error) {
	return nil, fmt.Errorf("the plan is already stored")
}
func (fakeHiringPlanner) DraftOutreach(context.Context, string, ai.Outreach) (string, error) {
	return "Hello", nil
}

// The screen drops internships and keeps the rest.
func (fakeHiringPlanner) ScreenHiringJobs(_ context.Context, _ string, in ai.HiringScreen) ([]ai.ProspectFit, error) {
	var fits []ai.ProspectFit
	for i, j := range in.Jobs {
		if !strings.Contains(j.Title, "Intern") {
			fits = append(fits, ai.ProspectFit{Index: i})
		}
	}
	return fits, nil
}
func (fakeHiringPlanner) ResearchCompany(context.Context, string, ai.CompanyResearch) (*ai.ResearchBrief, error) {
	return nil, nil
}

// fakeVerifier answers by profile URL.
type fakeVerifier map[string]*enrich.Result

func (f fakeVerifier) Enrich(_ context.Context, _ string, s enrich.Subject) (*enrich.Result, error) {
	return f[s.ProfileURL], nil
}

// DB-backed: one scan pulls the three sources through (fake) treg, runs the
// existing pipeline over them, stores each lead with its source and leaves a
// report in the workspace settings. The next scan asks only for newer postings.
func TestScanHiringSourcesDB(t *testing.T) {
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
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug, settings) VALUES ('hs-'||gen_random_uuid(), 'hs', 'hs-'||gen_random_uuid(),
		'{"hiring_signals":{"last_run":"2026-01-01T00:00:00Z","countries":["US"],"plan":{"applies":true,"roles":["software engineer"],"buyer_titles":"CTO or VP Engineering","max_company_size":5000,"why":"they need to screen engineers"}}}')
		RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)
	if _, err := pool.Exec(ctx, `INSERT INTO brand_profiles (workspace_id, product, overview, target_audience) VALUES ($1, 'Assessments', 'Technical skill assessments', 'Engineering leaders')`, ws); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	client, calls := fakeTreg(t, func(tool string, body map[string]any) (int, string) {
		switch tool {
		case tregHNHiringTool:
			return 200, fmt.Sprintf(`{"rows":[{"company":"Acme Robotics","posted":%q,"text":"Acme Robotics | Software Engineer | NYC | https://acmerobotics.test/jobs\nWe build robots.","url":"https://news.ycombinator.com/item?id=1"}]}`,
				now.Add(-time.Hour).Format(time.RFC3339))
		case tregBlitzJobsTool:
			posted := now.Add(-2 * time.Hour).Format("2006-01-02 15:04:05-07")
			row := func(company, title string) string {
				return fmt.Sprintf(`{"date_posted":%q,"title":%q,"url":"https://www.linkedin.com/jobs/view/%s","company_name":%q,"company_linkedin_url":"https://www.linkedin.com/company/x","location":{"city":null,"country_code":"US"}}`,
					posted, title, companyKey(company+title), company)
			}
			return 200, `{"results":[` + strings.Join([]string{row("Beta Labs", "Software Engineer"), row("Beta Labs", "Staff Software Engineer"),
				row("Giant Corp", "Software Engineer"), row("Intern Co", "Software Engineer Intern")}, ",") + `],"cursor":null}`
		case tregJobsTool:
			return 200, fmt.Sprintf(`{"output":{"data":{"items":[
			 {"company":"Giant Corp","title":"Software Engineer II","url":"https://www.linkedin.com/jobs/view/9","createdUtc":%[1]d},
			 {"company":"Gamma","title":"Software Engineer","url":"https://www.linkedin.com/jobs/view/10","createdUtc":%[1]d}]}}}`, now.Add(-3*time.Hour).Unix())
		case tregCompanyTool:
			switch body["name"] {
			case "Beta Labs": // size unknown; the filtered search already limited it
				return 200, `{"output":{"name":"Beta Labs","domain":"betalabs.test"}}`
			case "Giant Corp":
				return 200, `{"output":{"name":"Giant Corp","domain":"giant.test","employees":90000}}`
			case "Gamma":
				return 200, `{"output":{"name":"Gamma","domain":"gamma.test","employees":50}}`
			}
			return 200, `{"output":{}}` // Acme Robotics: the provider knows nothing
		case tregPeopleTool:
			q, _ := body["q"].(string)
			for company, slug := range map[string]string{"Acme Robotics": "ann", "Beta Labs": "bo"} {
				if strings.HasSuffix(q, " at "+company) {
					return 200, fmt.Sprintf(`{"output":{"people":[{"name":%q,"title":"CTO","company":%q,"profile_url":"https://www.linkedin.com/in/%s"}]}}`, slug, company, slug)
				}
			}
			return 200, `{"output":{"people":[]}}`
		}
		return 400, `{"error":"unexpected tool"}`
	})
	m := &Monitor{q: q, logger: zerolog.Nop(), treg: client, hiringPlanner: fakeHiringPlanner{}, contactVerifier: fakeVerifier{
		// The size comes from the contact's profile; the employer is checked
		// against the website the HN post named.
		"https://www.linkedin.com/in/ann": {FullName: "Ann", Title: "CTO", Company: "Acme Robotics", CompanyDomain: "acmerobotics.test", CompanySize: 120, Email: "ann@acmerobotics.test", EmailStatus: "verified"},
		// No size anywhere: confirmed because the source filtered by size.
		"https://www.linkedin.com/in/bo": {FullName: "Bo", Title: "CTO", Company: "Beta Labs", CompanyDomain: "betalabs.test", Email: "bo@betalabs.test", EmailStatus: "verified"},
	}}

	m.scanHiring(ctx, ws)

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
		if st.LastReport == nil {
			t.Fatalf("no report stored: %s", raw)
		}
		return st
	}
	st := state()
	rep := st.LastReport
	wantSources := map[string]sourceReport{
		sourceHN:       {Calls: 1, Fetched: 1, New: 1, Passed: 1, Stored: 1, Leads: 1},
		sourceBlitz:    {Calls: 1, Fetched: 4, New: 3, Passed: 2, Stored: 1, Leads: 1},
		sourceLinkedIn: {Calls: 1, Fetched: 2, New: 1, Passed: 1, Stored: 1, Leads: 0},
	}
	for name, want := range wantSources {
		if got := rep.Sources[name]; got == nil || *got != want {
			t.Errorf("%s: got %+v, want %+v", name, got, want)
		}
	}
	// Beta Labs' second posting and Giant Corp's second sighting; the
	// internship; Giant Corp's size; nobody found at Gamma.
	wantSkipped := map[string]int{"duplicate_or_incomplete": 2, "screened_out": 1, "too_big": 1, "no_contact_found": 1}
	if !reflect.DeepEqual(rep.Skipped, wantSkipped) {
		t.Errorf("skipped: got %v, want %v", rep.Skipped, wantSkipped)
	}
	if rep.SpendMicro != 3*660 || rep.Stopped != "" || st.LastScan.IsZero() || !reflect.DeepEqual(st.Countries, []string{"US"}) || !reflect.DeepEqual(st.Skip, []string{"giant-corp"}) {
		t.Errorf("state: spend=%d stopped=%q last_scan=%v countries=%v skip=%v", rep.SpendMicro, rep.Stopped, st.LastScan, st.Countries, st.Skip)
	}

	// Each stored signal and each lead carries the source that found it.
	rows, err := pool.Query(ctx, `SELECT m.author_username, m.platform_metadata->>'source', m.status::text, COALESCE(l.metadata->>'source', '')
		FROM mentions m LEFT JOIN leads l ON l.mention_id = m.id WHERE m.workspace_id = $1 ORDER BY 2`, ws)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var author, source, status, leadSource string
		if err := rows.Scan(&author, &source, &status, &leadSource); err != nil {
			t.Fatal(err)
		}
		got = append(got, strings.Join([]string{author, source, status, leadSource}, "|"))
	}
	rows.Close()
	want := []string{"bo|blitz_jobs|new|blitz_jobs", "ann|hn_hiring|new|hn_hiring", "Gamma|linkedin_jobs|archived|"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stored: got %v, want %v", got, want)
	}

	// The filtered search carried the workspace's countries and the plan's size.
	for _, c := range *calls {
		if c.tool == tregBlitzJobsTool {
			raw, _ := json.Marshal(c.body)
			if !strings.Contains(string(raw), `"country_code":{"include":["US"]}`) || !strings.Contains(string(raw), `"employee_count":{"max":5000,"min":20}`) {
				t.Errorf("filtered search body: %s", raw)
			}
		}
		if c.tool == tregJobsTool && c.body["postedLimit"] != "week" {
			t.Errorf("first scan asks LinkedIn for the week: %v", c.body)
		}
	}

	// An operator runs it again at once: only newer postings are asked for,
	// nothing is new, nothing is paid for twice.
	*calls = nil
	m.scanHiring(withForcedScan(ctx, ScanHiring), ws)
	rep = state().LastReport
	if hn := rep.Sources[sourceHN]; hn.Fetched != 0 {
		t.Errorf("the HN post is older than the last scan: %+v", hn)
	}
	if rep.Skipped["already_stored_this_month"] != 2 || rep.Skipped["on_skip_list"] != 1 {
		t.Errorf("second scan skipped: %v", rep.Skipped)
	}
	for _, c := range *calls {
		switch c.tool {
		case tregCompanyTool, tregPeopleTool:
			t.Errorf("second scan paid for a lookup: %+v", c)
		case tregJobsTool:
			if c.body["postedLimit"] != "24h" {
				t.Errorf("second scan asks LinkedIn for the last day: %v", c.body)
			}
		}
	}
}
