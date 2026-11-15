package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	"leadecho/internal/treg"
)

// The recorded answer (2026-11-15): three rounds, one dated two days ahead,
// investors listed beside the companies.
func TestParseFundingFixture(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	rounds, returned := parseFunding(fixture(t, "funding_rounds.json"), now)
	if returned != 3 || len(rounds) != 3 {
		t.Fatalf("want 3 of 3 rounds, got %d of %d", len(rounds), returned)
	}
	first := rounds[0]
	if first.Company != "Drillbit" || first.CompanyURL != "https://drillbit.com" || first.Title != "" || first.Source != sourceFunding ||
		first.URL != "https://zerogtalent.com/blog/drillbit-raises-4-4m-to-automate-field-service-operations-for-residential-contractors" ||
		first.CreatedUTC != time.Date(2026, 11, 13, 18, 22, 35, 0, time.UTC).Unix() {
		t.Errorf("first round: %+v", first)
	}
	if want := (fundingRound{Type: "seed", Amount: "$4.4 million", Date: "2026-11-13", URL: first.URL, domain: "drillbit.com"}); *first.Funding != want {
		t.Errorf("first round: got %+v, want %+v", *first.Funding, want)
	}
	// The legal form is dropped so the round and a posting share one lead;
	// the investors of the round are not companies that raised.
	last := rounds[2]
	if last.Company != "Tiberius Aerospace" || last.Funding.Type != "series_a" || last.Funding.label() != "Series A" || jobSiteDomain(last) != "tiberius.com" {
		t.Errorf("third round: %+v %+v", last, *last.Funding)
	}
	if got := last.Funding.fact(); got != "a Series A round of $85 million, announced 2026-11-17" {
		t.Errorf("fact: %q", got)
	}
	// The list is ordered by when a round was found, not by its date: a round
	// more than a few days ahead or older than two weeks is not news.
	if rounds, returned := parseFunding(fixture(t, "funding_rounds.json"), time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)); returned != 3 || len(rounds) != 2 {
		t.Errorf("a round dated five days ahead must be dropped: %d of %d", len(rounds), returned)
	}
	if rounds, returned := parseFunding(fixture(t, "funding_rounds.json"), time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)); returned != 3 || len(rounds) != 0 {
		t.Errorf("rounds older than 14 days must be dropped: %d of %d", len(rounds), returned)
	}
}

func TestParseFundingDropsUnusableRows(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	row := func(date, typ, amount, companyID, source string) string {
		return fmt.Sprintf(`{"attributes":{"effective_date":%q,"found_at":"2026-11-14T10:00:00Z","financing_type_normalized":%q,"amount":%q,"source_urls":[%q]},"relationships":{"company":{"data":{"id":%q}}}}`,
			date, typ, amount, source, companyID)
	}
	body := `{"output":{"data":[` + strings.Join([]string{
		row("2026-11-14", "seed", "$2m", "ok", "https://news.example/a"),
		row("2026-11-14", "seed", "$2m", "nodomain", "https://news.example/b"),
		row("2026-11-14", "seed", "$2m", "missing", "https://news.example/c"),
		row("2026-10-01", "series_b", "$30m", "ok", "https://news.example/d"),
		row("", "seed", "$2m", "ok", "https://news.example/e"),
		row("2026-11-14", "", "$2m", "ok", "https://news.example/f"),
		row("2026-11-14", "series_a", "", "linkedin", "https://news.example/g"),
		row("2026-11-14", "series_a", strings.Repeat("9", 300), "ok2", "javascript:alert(1)"),
	}, ",") + `],"included":[
	 {"id":"ok","attributes":{"domain":"https://www.Acme.io/about","company_name":"Acme, Inc."}},
	 {"id":"ok2","attributes":{"domain":"beta.example","company_name":"Beta"}},
	 {"id":"nodomain","attributes":{"domain":null,"company_name":"No Site"}},
	 {"id":"linkedin","attributes":{"domain":"linkedin.com","company_name":"Profile Only"}}]}}`
	rounds, returned := parseFunding([]byte(body), now)
	if returned != 8 || len(rounds) != 2 {
		t.Fatalf("want 2 of 8, got %d of %d: %+v", len(rounds), returned, rounds)
	}
	if a := rounds[0]; a.Company != "Acme" || a.Funding.domain != "acme.io" || a.CompanyURL != "https://acme.io" || a.Funding.URL != "https://news.example/a" {
		t.Errorf("first: %+v %+v", a, *a.Funding)
	}
	// Provider text is clipped, and a link that is not http(s) is not kept:
	// the lead then points at the company's own site.
	if b := rounds[1]; len(b.Funding.Amount) != maxFundingText || b.Funding.URL != "" || b.URL != "https://beta.example" || b.Funding.headline() == "" {
		t.Errorf("second: %+v %+v", b, *b.Funding)
	}
	if rounds, returned := parseFunding([]byte(`not json`), now); rounds != nil || returned != 0 {
		t.Error("bad json")
	}
}

func TestFundingLimit(t *testing.T) {
	n := func(v int) *int { return &v }
	for _, c := range []struct {
		set  *int
		want int
	}{{nil, defaultFundingLimit}, {n(0), 0}, {n(3), 3}, {n(-2), 0}, {n(500), maxFundingLimit}} {
		if got := fundingLimit(hiringState{FundingLimit: c.set}); got != c.want {
			t.Errorf("limit %v: got %d, want %d", c.set, got, c.want)
		}
	}
	if defaultFundingLimit != 5 {
		t.Error("the default is 5 records a day ($0.20)")
	}
}

// One GET through the treg client, with the day's limit and the price of
// that many records as the call's ceiling; not bought twice in a day.
func TestFundedCompaniesCall(t *testing.T) {
	now := time.Now().UTC()
	var reqs []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, r)
		w.Header().Set("X-Treg-Cost-Micro", "120000")
		fmt.Fprintf(w, `{"data":[{"attributes":{"effective_date":%q,"found_at":%q,"financing_type_normalized":"seed","amount":"$3m","source_urls":["https://news.example/x"]},"relationships":{"company":{"data":{"id":"c1"}}}}],
		 "included":[{"id":"c1","attributes":{"domain":"acme.example","company_name":"Acme"}}]}`, now.Format(time.DateOnly), now.Add(-time.Hour).Format(time.RFC3339))
	}))
	defer srv.Close()
	m := &Monitor{logger: zerolog.Nop(), treg: treg.New(treg.Config{Token: "t", BaseURL: srv.URL, MaxCostMicro: 20000}, nil, zerolog.Nop())}
	three := 3
	st := hiringState{FundingLimit: &three, Countries: []string{"us", "CA"}}
	rep := &hiringReport{}
	got := m.fundedCompanies(context.Background(), testWS, &st, now, rep)
	if len(got) != 1 || got[0].Company != "Acme" || len(reqs) != 1 {
		t.Fatalf("got %+v after %d calls", got, len(reqs))
	}
	r := reqs[0]
	q := r.URL.Query()
	if r.Method != http.MethodGet || r.URL.Path != "/call/predictleads.financing.discover" || q.Get("financing_types_normalized") != "seed,series_a,series_b" ||
		q.Get("company_location") != "United States" || q.Get("limit") != "3" || len(q) != 3 {
		t.Errorf("request: %s %s", r.Method, r.URL)
	}
	if got := r.Header.Get("X-Treg-Route-Max-Cost"); got != "0.12" { // 3 records at $0.04
		t.Errorf("ceiling %q, want 0.12", got)
	}
	if r.Header.Get("X-Treg-Meta") != "customer="+testWS {
		t.Error("the call must be tagged with the workspace, so its daily cap applies")
	}
	if sr := rep.Sources[sourceFunding]; *sr != (sourceReport{Calls: 1, Fetched: 1}) || rep.FundingSpendMicro != 120000 || !st.LastFunding.Equal(now) {
		t.Errorf("report %+v spend %d last %v", *sr, rep.FundingSpendMicro, st.LastFunding)
	}
	// Again the same day (a retry, an operator's manual scan): not bought twice.
	if got := m.fundedCompanies(context.Background(), testWS, &st, now.Add(time.Hour), rep); got != nil || len(reqs) != 1 {
		t.Errorf("second call the same day: %+v, %d calls", got, len(reqs))
	}
	// Off by setting, and for a workspace that does not sell in the US.
	zero := 0
	for name, st := range map[string]hiringState{"limit 0": {FundingLimit: &zero}, "no US": {Countries: []string{"IN"}}} {
		if got := m.fundedCompanies(context.Background(), testWS, &st, now, &hiringReport{}); got != nil || len(reqs) != 1 {
			t.Errorf("%s: %+v, %d calls", name, got, len(reqs))
		}
	}
}

// A failed call buys nothing and is reported; the next run tries again.
func TestFundedCompaniesFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad_request"}`))
	}))
	defer srv.Close()
	m := &Monitor{logger: zerolog.Nop(), treg: treg.New(treg.Config{Token: "t", BaseURL: srv.URL}, nil, zerolog.Nop())}
	st, rep := hiringState{}, &hiringReport{}
	if got := m.fundedCompanies(context.Background(), testWS, &st, time.Now(), rep); got != nil || !st.LastFunding.IsZero() || rep.Sources[sourceFunding].FailedCalls != 1 {
		t.Errorf("got %+v, last %v, report %+v", got, st.LastFunding, rep.Sources[sourceFunding])
	}
}

func TestMergeFunded(t *testing.T) {
	round := func(company string) jobPost {
		return jobPost{Company: company, Source: sourceFunding, Funding: &fundingRound{Type: "seed", Date: "2026-11-13", domain: companyKey(company) + ".example"}}
	}
	fresh := []jobPost{{Company: "Acme", Title: "Engineer", Source: sourceBlitz}, {Company: "Beta Labs", Title: "Engineer", Source: sourceHN}}
	rep := &hiringReport{}
	got := mergeFunded(fresh, []jobPost{round("Gamma"), round("beta labs"), round("Stored Co"), round("Gamma")}, func(j jobPost) string {
		if j.Company == "Stored Co" {
			return "already_stored_this_month"
		}
		return ""
	}, rep)
	// The funded company with no posting goes first; the one with a posting
	// is that posting, carrying the round; a stored one is skipped.
	if len(got) != 3 || got[0].Company != "Gamma" || signalKind(got[0]) != "funding" || got[1].Company != "Acme" || got[1].Funding != nil ||
		got[2].Company != "Beta Labs" || got[2].Funding == nil || signalKind(got[2]) != "hiring" || got[2].Source != sourceHN {
		t.Fatalf("got %+v", got)
	}
	if want := map[string]int{"already_stored_this_month": 1, "duplicate_or_incomplete": 1}; !reflect.DeepEqual(rep.Skipped, want) || rep.Sources[sourceFunding].New != 1 {
		t.Errorf("report: %v %+v", rep.Skipped, rep.Sources[sourceFunding])
	}
	if got := mergeFunded(fresh, nil, nil, rep); len(got) != 2 {
		t.Error("no rounds: the postings are returned as they are")
	}
}

// What the lead says and stores comes from the round's own fields.
func TestFundingSignalTextAndMetadata(t *testing.T) {
	f := &fundingRound{Type: "series_a", Amount: "$85 million", Date: "2026-11-17", URL: "https://news.example/round", domain: "acme.example"}
	funded := jobPost{Company: "Acme", CompanyURL: "https://acme.example", URL: f.URL, Source: sourceFunding, Funding: f, site: "www.acme.example"}
	title, opening := signalText(funded)
	if title != "Acme raised a Series A round of $85 million" || opening != "Acme raised a Series A round of $85 million, announced 2026-11-17." {
		t.Errorf("funding: %q / %q", title, opening)
	}
	var meta map[string]any
	if err := json.Unmarshal(hiringMetadata(funded, "CTO", contactCheck{confirmed: true, email: "verified"}, nil), &meta); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"signal": "funding", "company": "Acme", "company_url": "https://acme.example", "company_domain": "acme.example", "contact_title": "CTO",
		"confirmed": true, "unconfirmed_reason": "", "email_status": "verified", "source": "funding",
		"funding_type": "series_a", "funding_amount": "$85 million", "funding_date": "2026-11-17", "funding_url": "https://news.example/round"}
	if !reflect.DeepEqual(meta, want) {
		t.Errorf("metadata:\n got %v\nwant %v", meta, want)
	}

	// Raised and hiring: a hiring signal that also carries the round.
	both := jobPost{Company: "Acme", Title: "Backend Engineer", Location: "Austin, TX", URL: "https://www.linkedin.com/jobs/view/1", Source: sourceLinkedIn, Funding: f}
	title, opening = signalText(both)
	if title != "Acme is hiring: Backend Engineer" || opening != `Acme posted a job for "Backend Engineer" (Austin, TX). It also raised a Series A round of $85 million, announced 2026-11-17.` {
		t.Errorf("both: %q / %q", title, opening)
	}
	meta = nil
	_ = json.Unmarshal(hiringMetadata(both, "CTO", contactCheck{}, nil), &meta)
	if meta["signal"] != "hiring" || meta["job_title"] != "Backend Engineer" || meta["job_url"] != both.URL || meta["funding_type"] != "series_a" || meta["funding_url"] != f.URL {
		t.Errorf("both: %v", meta)
	}
	// A plain posting stores no funding keys, and reads as before.
	plain := jobPost{Company: "Acme", Title: "Backend Engineer", URL: "https://www.linkedin.com/jobs/view/1"}
	if title, opening := signalText(plain); title != "Acme is hiring: Backend Engineer" || opening != `Acme posted a job for "Backend Engineer".` {
		t.Errorf("plain: %q / %q", title, opening)
	}
	// The stored text is read for weeks: it carries the posting's date, never "this week".
	plain.CreatedUTC = time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC).Unix()
	if _, opening := signalText(plain); opening != `Acme posted a job for "Backend Engineer" on 12 November 2026.` {
		t.Errorf("dated: %q", opening)
	}
	plain.CreatedUTC = 0
	if raw := string(hiringMetadata(plain, "", contactCheck{}, nil)); strings.Contains(raw, "funding") {
		t.Errorf("plain: %s", raw)
	}
	// No amount announced.
	if got := (&fundingRound{Type: "seed", Date: "2026-11-13"}).fact(); got != "a Seed round, announced 2026-11-13" {
		t.Errorf("no amount: %q", got)
	}
	// The second fact for a lead already stored: a round only where the
	// websites agree, a posting by its own keys.
	if patch, marker, domain := signalFact(funded); marker != "funding_type" || domain != "acme.example" || patch["funding_amount"] != "$85 million" {
		t.Errorf("round fact: %v %q %q", patch, marker, domain)
	}
	if patch, marker, domain := signalFact(both); marker != "job_title" || domain != "" || patch["job_title"] != "Backend Engineer" || patch["job_url"] != both.URL {
		t.Errorf("posting fact: %v %q %q", patch, marker, domain)
	}
	if got := screenInput([]jobPost{funded, plain}); got[0].Funding != f.fact() || got[0].Website != "acme.example" || got[0].SourceURL != f.URL || got[1].Funding != "" {
		t.Errorf("screen input: %+v", got)
	}
}

// recordingPlanner keeps what the writer was told.
type recordingPlanner struct {
	fakeHiringPlanner
	drafts map[string]ai.Outreach
}

func (p recordingPlanner) DraftOutreach(_ context.Context, _ string, o ai.Outreach) (string, error) {
	p.drafts[o.Company] = o
	return "Hello", nil
}

// DB-backed: a day's rounds go through the scan beside its postings. A
// funded company becomes a funding lead; one that is also hiring is ONE lead
// carrying both facts, whichever was found first.
func TestScanFundingDB(t *testing.T) {
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
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug, settings) VALUES ('fs-'||gen_random_uuid(), 'fs', 'fs-'||gen_random_uuid(),
		'{"hiring_signals":{"last_run":"2026-01-01T00:00:00Z","funding_limit":2,"plan":{"applies":true,"roles":["software engineer"],"buyer_titles":"CTO or VP Engineering","max_company_size":5000,"why":"they need to screen engineers"}}}')
		RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)
	if _, err := pool.Exec(ctx, `INSERT INTO brand_profiles (workspace_id, product, overview, target_audience) VALUES ($1, 'Assessments', 'Technical skill assessments', 'Engineering leaders')`, ws); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	slug := func(company string) string { return companyKey(company) }
	domain := func(company string) string { return strings.ReplaceAll(slug(company), "-", "") + ".test" }
	rounds := func(companies map[string]string) string { // company -> website the provider gives
		var data, included []string
		for company, site := range companies {
			data = append(data, fmt.Sprintf(`{"attributes":{"effective_date":%q,"found_at":%q,"financing_type_normalized":"seed","amount":"$8m","source_urls":["https://news.test/%s"]},"relationships":{"company":{"data":{"id":%q}}}}`,
				now.Format(time.DateOnly), now.Add(-time.Hour).Format(time.RFC3339), slug(company), company))
			included = append(included, fmt.Sprintf(`{"id":%q,"attributes":{"domain":%q,"company_name":%q}}`, company, site, company+" Inc."))
		}
		return `{"data":[` + strings.Join(data, ",") + `],"included":[` + strings.Join(included, ",") + `]}`
	}
	postings := func(companies ...string) string {
		var rows []string
		for _, c := range companies {
			rows = append(rows, fmt.Sprintf(`{"date_posted":%q,"title":"Software Engineer","url":"https://www.linkedin.com/jobs/view/%s","company_name":%q,"company_linkedin_url":"https://www.linkedin.com/company/%s","location":{"city":"Austin","country_code":"US"}}`,
				now.Add(-2*time.Hour).Format("2006-01-02 15:04:05-07"), slug(c), c, slug(c)))
		}
		return `{"results":[` + strings.Join(rows, ",") + `],"cursor":null}`
	}
	var blitz, funding string
	fundingCalls := 0
	client, _ := fakeTreg(t, func(tool string, body map[string]any) (int, string) {
		switch tool {
		case tregHNHiringTool:
			return 200, `{"rows":[]}`
		case tregJobsTool:
			return 200, `{"output":{"data":{"items":[]}}}`
		case tregBlitzJobsTool:
			return 200, blitz
		case tregFundingTool:
			fundingCalls++
			return 200, funding
		case tregCompanyTool:
			name, _ := body["name"].(string)
			return 200, fmt.Sprintf(`{"output":{"name":%q,"domain":%q,"employees":60}}`, name, domain(name))
		case tregPeopleTool:
			q, _ := body["q"].(string)
			_, company, _ := strings.Cut(q, " at ")
			return 200, fmt.Sprintf(`{"output":{"people":[{"name":%q,"title":"CTO","company":%q,"profile_url":"https://www.linkedin.com/in/%s"}]}}`, "Cto "+company, company, slug(company))
		}
		return 400, `{"error":"unexpected tool"}`
	})
	verifier := fakeVerifier{}
	for _, c := range []string{"Beta Labs", "Delta AI", "Gamma", "Omega"} {
		verifier["https://www.linkedin.com/in/"+slug(c)] = &enrich.Result{FullName: "Cto " + c, Title: "CTO", Company: c, CompanyDomain: domain(c),
			Email: "cto@" + domain(c), EmailStatus: "verified"}
	}
	planner := recordingPlanner{drafts: map[string]ai.Outreach{}}
	m := &Monitor{q: q, logger: zerolog.Nop(), treg: client, hiringPlanner: planner, contactVerifier: verifier}
	report := func() *hiringReport {
		t.Helper()
		raw, err := q.GetWorkspaceSettingsKey(ctx, ws, hiringSettingsKey)
		if err != nil {
			t.Fatal(err)
		}
		var st hiringState
		if err := json.Unmarshal(raw, &st); err != nil || st.LastReport == nil {
			t.Fatalf("state: %v %s", err, raw)
		}
		return st.LastReport
	}
	stored := func() map[string]string { // company -> what its one lead carries
		t.Helper()
		rows, err := pool.Query(ctx, `SELECT m.platform_metadata->>'company', m.platform_metadata->>'signal', m.title, m.url, m.status::text,
			COALESCE(m.platform_metadata->>'funding_type','') || ' ' || COALESCE(m.platform_metadata->>'funding_amount','') || ' ' || COALESCE(m.platform_metadata->>'funding_url',''),
			COALESCE(m.platform_metadata->>'job_title',''), COALESCE(array_to_string(l.tags, ','), '')
			FROM mentions m LEFT JOIN leads l ON l.mention_id = m.id WHERE m.workspace_id = $1`, ws)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[string]string{}
		for rows.Next() {
			var f [8]string
			if err := rows.Scan(&f[0], &f[1], &f[2], &f[3], &f[4], &f[5], &f[6], &f[7]); err != nil {
				t.Fatal(err)
			}
			if _, dup := out[f[0]]; dup {
				t.Errorf("%s has two leads", f[0])
			}
			out[f[0]] = strings.Join(f[1:], " | ")
		}
		return out
	}

	// Day 1: Beta Labs posted a job and raised; Delta AI only raised.
	blitz = postings("Beta Labs")
	funding = rounds(map[string]string{"Beta Labs": "betalabs.test", "Delta AI": "https://www.deltaai.test/"})
	m.scanHiring(ctx, ws)
	want := map[string]string{
		"Beta Labs": "hiring | Beta Labs is hiring: Software Engineer | https://www.linkedin.com/jobs/view/beta-labs | new | seed $8m https://news.test/beta-labs | Software Engineer | auto-qualified,hiring-signal",
		"Delta AI":  "funding | Delta AI raised a Seed round of $8m | https://news.test/delta-ai | new | seed $8m https://news.test/delta-ai |  | auto-qualified,funding-signal",
	}
	if got := stored(); !reflect.DeepEqual(got, want) {
		t.Fatalf("day 1:\n got %v\nwant %v", got, want)
	}
	rep := report()
	if got := rep.Sources[sourceFunding]; got == nil || *got != (sourceReport{Calls: 1, Fetched: 2, New: 1, Passed: 1, Stored: 1, Leads: 1}) || rep.FundingSpendMicro != 660 {
		t.Errorf("funding in the report: %+v spend %d", got, rep.FundingSpendMicro)
	}
	// The funding lead's draft opens on the round; the other's on the job, with the round beside it.
	round := "a Seed round of $8m, announced " + now.Format(time.DateOnly)
	if d := planner.drafts["Delta AI"]; d.Funding != round || d.JobTitle != "" || d.ContactName != "Cto Delta AI" {
		t.Errorf("funding draft input: %+v", d)
	}
	if d := planner.drafts["Beta Labs"]; d.Funding != round || d.JobTitle != "Software Engineer" || d.JobLocation != "Austin, US" {
		t.Errorf("hiring draft input: %+v", d)
	}
	var content string
	if err := pool.QueryRow(ctx, `SELECT content FROM mentions WHERE workspace_id = $1 AND platform_metadata->>'signal' = 'funding'`, ws).Scan(&content); err != nil ||
		!strings.HasPrefix(content, "Delta AI raised a Seed round of $8m, announced "+now.Format(time.DateOnly)+".\n\nWho to contact: Cto Delta AI, CTO at Delta AI.") {
		t.Errorf("funding lead text: %v %q", err, content)
	}
	confirmed, held, err := q.DigestHiringSignals(ctx, ws, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil || confirmed != 2 || held != 0 {
		t.Errorf("digest counts funding leads with hiring leads: %d confirmed, %d held, %v", confirmed, held, err)
	}

	// Later the same day (an operator's scan): Delta AI posts a job, Gamma and
	// Omega post theirs. The rounds are not bought again.
	blitz = postings("Delta AI", "Gamma", "Omega")
	m.scanHiring(withForcedScan(ctx, ScanHiring), ws)
	if fundingCalls != 1 {
		t.Errorf("rounds bought %d times in a day", fundingCalls)
	}
	got := stored()
	if len(got) != 4 || got["Delta AI"] != "funding | Delta AI raised a Seed round of $8m | https://news.test/delta-ai | new | seed $8m https://news.test/delta-ai | Software Engineer | auto-qualified,funding-signal" {
		t.Fatalf("the posting must be added to the funding lead:\n%v", got)
	}

	// Next day: Gamma raised (its website is the one its lead was confirmed
	// against); so did a company called Omega with another website.
	if _, err := pool.Exec(ctx, `UPDATE workspaces SET settings = jsonb_set(settings, '{hiring_signals,last_funding}', '"2026-01-01T00:00:00Z"') WHERE id = $1`, ws); err != nil {
		t.Fatal(err)
	}
	blitz = postings()
	funding = rounds(map[string]string{"Gamma": "gamma.test", "Omega": "omega-holdings.test"})
	m.scanHiring(withForcedScan(ctx, ScanHiring), ws)
	got = stored()
	if len(got) != 4 || !strings.Contains(got["Gamma"], "hiring | Gamma is hiring: Software Engineer") || !strings.Contains(got["Gamma"], "seed $8m https://news.test/gamma | Software Engineer") {
		t.Errorf("the round must be added to Gamma's hiring lead: %q", got["Gamma"])
	}
	if strings.Contains(got["Omega"], "seed") {
		t.Errorf("a namesake's round must not be stated on Omega's lead: %q", got["Omega"])
	}
	if rep := report(); rep.Skipped["already_stored_this_month"] != 2 || rep.Sources[sourceFunding].Fetched != 2 || rep.Sources[sourceFunding].New != 0 {
		t.Errorf("day 2 report: %v %+v", rep.Skipped, rep.Sources[sourceFunding])
	}
}
