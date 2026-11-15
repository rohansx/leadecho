package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/ai"
	"leadecho/internal/enrich"
	"leadecho/internal/treg"
)

// testdata/blitz_jobs.json and testdata/hn_hiring.json are real responses
// recorded on 2026-11-15.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseBlitzJobs(t *testing.T) {
	jobs, cursor := parseBlitzJobs(fixture(t, "blitz_jobs.json"), true)
	if len(jobs) != 25 || cursor == "" {
		t.Fatalf("want 25 postings and a cursor, got %d, cursor %q", len(jobs), cursor)
	}
	first := jobs[0]
	want := jobPost{Company: "Voyager Technologies", CompanyURL: "https://www.linkedin.com/company/voyagertechnologies-inc",
		Title: "Senior Build & Release Software Engineer", Location: "Pittsburgh, US",
		URL:        "https://www.linkedin.com/jobs/view/senior-build-release-software-engineer-at-voyager-technologies-4491218423",
		CreatedUTC: time.Date(2026, 11, 15, 3, 27, 4, 0, time.UTC).Unix(), Source: sourceBlitz, SizeFits: true}
	if first != want {
		t.Fatalf("first posting:\n got %+v\nwant %+v", first, want)
	}
	// "2026-11-15 00:24:11.585+01" (fractional seconds) and a null city.
	if jobs[1].CreatedUTC != time.Date(2026, 11, 14, 23, 24, 11, 0, time.UTC).Unix() || jobs[1].Location != "US" {
		t.Fatalf("second posting: %+v", jobs[1])
	}
	for _, j := range jobs {
		if j.CreatedUTC == 0 || !validHTTPURL(j.URL) || j.Company == "" {
			t.Fatalf("unusable posting: %+v", j)
		}
	}
	// 25 postings, 10 employers: one company posts many jobs.
	if got := onePerCompany(jobs); len(got) != 10 {
		t.Fatalf("want 10 companies, got %d", len(got))
	}
	if jobs, _ := parseBlitzJobs([]byte(`{"output":{"results":[{"company_name":"A","url":"https://a.test/j"}],"cursor":null}}`), false); len(jobs) != 1 || jobs[0].SizeFits {
		t.Fatalf("wrapped body: %+v", jobs)
	}
	if jobs, cursor := parseBlitzJobs([]byte(`not json`), true); jobs != nil || cursor != "" {
		t.Fatal("bad json should give nothing")
	}
}

func TestBlitzJobsBody(t *testing.T) {
	plan := &ai.HiringPlan{Roles: []string{"software engineer", "backend engineer"}, MaxCompanySize: 5000}
	asJSON := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	// The body that worked in the live probe.
	want := `{"company":{"employee_count":{"max":5000,"min":20},"is_agency":false},"job":{"date_posted":{"last_days":2},"location":{"country_code":{"include":["US"]}},"title":{"include":["software engineer","backend engineer"]}},"max_results":25}`
	if got := asJSON(blitzJobsBody(plan, []string{" us ", "usa", ""}, true, 25, "")); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	// No countries set: no location filter. No size ceiling: no size filter.
	// Not run yesterday: the last week. A cursor asks for the next page.
	got := asJSON(blitzJobsBody(&ai.HiringPlan{Roles: []string{"designer"}}, nil, false, 10, "abc"))
	want = `{"company":{"is_agency":false},"cursor":"abc","job":{"date_posted":{"last_days":7},"title":{"include":["designer"]}},"max_results":10}`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestParseHNHiring(t *testing.T) {
	body := fixture(t, "hn_hiring.json")
	kws := []string{"engineer"}
	all := parseHNHiring(body, "engineer", kws, time.Time{})
	if len(all) < 120 || len(all) > 157 {
		t.Fatalf("want most of the 157 posts, got %d", len(all))
	}
	by := map[string]jobPost{}
	for _, j := range all {
		if j.Source != sourceHN || j.CreatedUTC == 0 || !strings.HasPrefix(j.URL, "https://news.ycombinator.com/item?id=") ||
			j.Company == "" || j.Title == "" || len([]rune(j.Title)) > maxHNField || len([]rune(j.Location)) > maxHNField {
			t.Fatalf("unusable posting: %+v", j)
		}
		by[j.Company] = j
	}
	// "Simple Analytics | DevOps Engineer | REMOTE (...) | ... | https://www.simpleanalytics.com/jobs/devops"
	sa := by["Simple Analytics"]
	if sa.Title != "DevOps Engineer" || sa.CompanyURL != "https://www.simpleanalytics.com" || !strings.HasPrefix(sa.Location, "REMOTE (worldwide") ||
		sa.CreatedUTC != time.Date(2026, 11, 2, 16, 1, 43, 0, time.UTC).Unix() {
		t.Fatalf("Simple Analytics: %+v", sa)
	}
	// "PostHog | Full-Time | Technical CSMs, SRE, Product Manager, AI Research Engineer | ...":
	// the role is the field that names it, not the second field.
	if got := by["PostHog"].Title; got != "Technical CSMs, SRE, Product Manager, AI Research Engineer" {
		t.Fatalf("PostHog title: %q", got)
	}
	if !strings.HasPrefix(by["PostHog"].Location, "Full-Time, REMOTE") {
		t.Fatalf("PostHog location: %q", by["PostHog"].Location)
	}
	// "SwingVision | Berkeley, CA | Full time + Internships": no field names a
	// role, so the role is the word the thread was searched by.
	if sv := by["SwingVision"]; sv.Title != "engineer" || sv.Location != "Berkeley, CA, Full time + Internships" {
		t.Fatalf("SwingVision: %+v", sv)
	}
	// "MixRank (YC S11)": the batch note is not part of the name.
	if _, ok := by["MixRank"]; !ok {
		t.Fatal("want MixRank without its bracketed note")
	}
	// Equall links only to its applicant-tracking page: not a company site.
	if e, ok := by["Equall"]; !ok || e.CompanyURL != "" {
		t.Fatalf("Equall: %+v", e)
	}
	// Only posts made since the last scan are considered.
	since := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	recent := parseHNHiring(body, "engineer", kws, since)
	if len(recent) == 0 || len(recent) > 8 {
		t.Fatalf("8 posts were made since the 13th, got %d", len(recent))
	}
	for _, j := range recent {
		if j.CreatedUTC <= since.Unix() {
			t.Fatalf("older than the last scan: %+v", j)
		}
	}
	if parseHNHiring([]byte(`not json`), "engineer", kws, time.Time{}) != nil {
		t.Fatal("bad json should give nothing")
	}
	one := `{"output":{"rows":[{"company":null,"posted":"2026-11-14T10:00:00Z","text":"Acme | Rust Engineer | Berlin\nmore","url":"https://news.ycombinator.com/item?id=1"},
	 {"company":"NoRole","posted":"2026-11-14T10:00:00Z","text":"NoRole","url":"https://news.ycombinator.com/item?id=2"},
	 {"company":"Old","posted":"bad date","text":"Old | Engineer","url":"https://news.ycombinator.com/item?id=3"}]}}`
	got := parseHNHiring([]byte(one), "engineer", kws, time.Time{})
	if len(got) != 1 || got[0].Company != "Acme" || got[0].Title != "Rust Engineer" || got[0].Location != "Berlin" {
		t.Fatalf("wrapped body, missing company, no role, bad date: %+v", got)
	}
}

func TestCompanySiteURL(t *testing.T) {
	for text, want := range map[string]string{
		"Acme | Engineer | https://jobs.ashbyhq.com/acme/1 see https://Acme.io/careers.": "https://acme.io",
		"apply at https://boards.greenhouse.io/acme":                                     "",
		"no link here":                  "",
		"(https://eng.acme.co.uk/jobs)": "https://eng.acme.co.uk",
		"javascript://x ftp://y.test":   "",
	} {
		if got := companySiteURL(text); got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
	if d := jobSiteDomain(jobPost{Source: sourceHN, CompanyURL: "https://www.acme.io"}); d != "acme.io" {
		t.Errorf("an HN post's site is the company's domain, got %q", d)
	}
	if d := jobSiteDomain(jobPost{Source: sourceLinkedIn, CompanyURL: "https://www.linkedin.com/company/acme"}); d != "" {
		t.Errorf("a LinkedIn company page is not the company's website, got %q", d)
	}
}

func TestRoleKeywordsAndCountryCodes(t *testing.T) {
	got := roleKeywords([]string{"senior backend engineer", "Software Engineer", "full-stack developer", "  ", "qa"})
	if want := []string{"engineer", "developer"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := countryCodes([]string{"us", " GB", "Germany", "1A", ""}); !reflect.DeepEqual(got, []string{"US", "GB"}) {
		t.Fatalf("got %v", got)
	}
}

// fakeTreg answers /call/<tool> with handler(tool, body) and records the calls.
type tregCall struct {
	tool string
	body map[string]any
}

func fakeTreg(t *testing.T, handler func(tool string, body map[string]any) (status int, resp string)) (*treg.Client, *[]tregCall) {
	t.Helper()
	var calls []tregCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		tool := strings.TrimPrefix(r.URL.Path, "/call/")
		calls = append(calls, tregCall{tool, body})
		status, resp := handler(tool, body)
		w.Header().Set("X-Treg-Cost-Micro", "660")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return treg.New(treg.Config{Token: "t", BaseURL: srv.URL, MaxCostMicro: 20000}, nil, zerolog.Nop()), &calls
}

const testWS = "11111111-2222-3333-4444-555555555555"

func linkedInPage(company, cursor string) string {
	return fmt.Sprintf(`{"output":{"data":{"items":[{"company":%q,"title":"Engineer","url":"https://www.linkedin.com/jobs/view/1","createdUtc":%d}],"nextCursor":%q}}}`,
		company, time.Now().Unix(), cursor)
}

// Every role's first page before any role's second; the cursor asks for the
// next page; a role with no cursor left is finished.
func TestLinkedInSourcePages(t *testing.T) {
	client, calls := fakeTreg(t, func(_ string, body map[string]any) (int, string) {
		role, cursor := body["query"].(string), ""
		if role == "a" && body["cursor"] == nil {
			cursor = "a2" // role a has a second page, role b does not
		}
		return 200, linkedInPage(role+fmt.Sprint(body["cursor"]), cursor)
	})
	m := &Monitor{logger: zerolog.Nop(), treg: client}
	src := m.linkedInSource(testWS, []string{"a", "b"}, true)
	if src.maxPages != 2*linkedInPagesPerRole {
		t.Fatalf("maxPages = %d", src.maxPages)
	}
	var companies []string
	for i := 0; i < src.maxPages; i++ {
		p, err := src.next(context.Background(), 5)
		if err != nil {
			t.Fatal(err)
		}
		for _, j := range p.jobs {
			if j.Source != sourceLinkedIn {
				t.Fatalf("untagged: %+v", j)
			}
			companies = append(companies, j.Company)
		}
		if p.costMicro != 660 {
			t.Fatalf("cost not reported: %+v", p)
		}
		if !p.more {
			break
		}
	}
	if want := []string{"a<nil>", "b<nil>", "aa2"}; !reflect.DeepEqual(companies, want) {
		t.Fatalf("got %v, want %v", companies, want)
	}
	for _, c := range *calls {
		if c.tool != tregJobsTool || c.body["postedLimit"] != "24h" {
			t.Fatalf("call: %+v", c)
		}
	}
	// A failed role is given up; the other role is still searched.
	client, _ = fakeTreg(t, func(_ string, body map[string]any) (int, string) {
		if body["query"] == "a" {
			return 400, `{"error":"bad"}`
		}
		return 200, linkedInPage("B", "")
	})
	m = &Monitor{logger: zerolog.Nop(), treg: client}
	src = m.linkedInSource(testWS, []string{"a", "b"}, false)
	if p, err := src.next(context.Background(), 5); err == nil || !p.more {
		t.Fatalf("first role fails, the second is left: %+v %v", p, err)
	}
	if p, err := src.next(context.Background(), 5); err != nil || len(p.jobs) != 1 || p.more {
		t.Fatalf("second role: %+v %v", p, err)
	}
}

// The sources ask for the last day when the last scan was yesterday, for the
// last week otherwise, and the filtered search sizes its page under the
// per-call cost ceiling.
func TestHiringSourcesWindow(t *testing.T) {
	now := time.Now().UTC()
	st := hiringState{Plan: &ai.HiringPlan{Roles: []string{"software engineer"}, MaxCompanySize: 5000}, Countries: []string{"US"}}
	run := func(since time.Time, wantHN int) []tregCall {
		client, calls := fakeTreg(t, func(tool string, _ map[string]any) (int, string) {
			if tool == tregHNHiringTool {
				return 200, fmt.Sprintf(`{"rows":[{"company":"Old Co","posted":%q,"text":"Old Co | Engineer","url":"https://news.ycombinator.com/item?id=1"},
				 {"company":"New Co","posted":%q,"text":"New Co | Engineer","url":"https://news.ycombinator.com/item?id=2"}]}`,
					now.Add(-48*time.Hour).Format(time.RFC3339), now.Add(-time.Hour).Format(time.RFC3339))
			}
			return 200, `{}`
		})
		m := &Monitor{logger: zerolog.Nop(), treg: client}
		rep := &hiringReport{}
		fresh, searched := m.collectHiringJobs(context.Background(), m.hiringSources(testWS, st, since, now), 100, func(jobPost) string { return "" }, rep)
		if !searched {
			t.Fatal("searches succeeded")
		}
		if len(fresh) != wantHN || rep.Sources[sourceHN].New != wantHN {
			t.Fatalf("since %v: want %d HN postings, got %+v", since, wantHN, fresh)
		}
		return *calls
	}
	for _, tc := range []struct {
		since  time.Time
		posted string
		days   float64
		hn     int // of the two HN posts (48h and 1h old), those newer than the last scan
	}{
		{time.Time{}, "week", 7, 2}, // first run: the last week
		{now.Add(-24 * time.Hour), "24h", 2, 1},
		{now.Add(-72 * time.Hour), "week", 7, 2},
	} {
		calls := run(tc.since, tc.hn)
		if len(calls) != 3 || calls[0].tool != tregHNHiringTool || calls[1].tool != tregBlitzJobsTool || calls[2].tool != tregJobsTool {
			t.Fatalf("want HN, then the filtered search, then LinkedIn: %+v", calls)
		}
		if calls[0].body["keyword"] != "engineer" || calls[0].body["remote"] != false {
			t.Fatalf("HN call: %+v", calls[0].body)
		}
		job := calls[1].body["job"].(map[string]any)
		if job["date_posted"].(map[string]any)["last_days"] != tc.days || calls[2].body["postedLimit"] != tc.posted {
			t.Fatalf("since %v: %+v / %+v", tc.since, calls[1].body, calls[2].body)
		}
		// A $0.02 ceiling at $0.001 a posting: 20 postings a page, not 50.
		if calls[1].body["max_results"] != float64(20) {
			t.Fatalf("max_results = %v", calls[1].body["max_results"])
		}
	}
}

func pagesOf(name string, pages ...[]jobPost) jobSource {
	i := 0
	return jobSource{name: name, maxPages: 10, next: func(context.Context, int) (jobPage, error) {
		p := jobPage{jobs: pages[i], more: i+1 < len(pages), costMicro: 1000}
		i++
		return p, nil
	}}
}

func post(company string) jobPost {
	return jobPost{Company: company, Title: "Engineer", URL: "https://jobs.test/" + company}
}

func TestCollectHiringJobs(t *testing.T) {
	m := &Monitor{logger: zerolog.Nop()}
	skipList := func(j jobPost) string {
		if strings.HasPrefix(j.Company, "Giant") {
			return "on_skip_list"
		}
		return ""
	}
	sources := func() []jobSource {
		return []jobSource{
			pagesOf("first",
				[]jobPost{post("Giant1"), post("Giant2"), post("A"), post("A"), {Company: "NoURL"}},
				[]jobPost{post("Giant3"), post("A"), post("B")},
				[]jobPost{post("C"), post("D")}),
			pagesOf("second", []jobPost{post("B"), post("E"), post("F")}),
		}
	}

	// The first page is mostly known giants: keep pulling until the target.
	rep := &hiringReport{}
	fresh, searched := m.collectHiringJobs(context.Background(), sources(), 3, skipList, rep)
	var names []string
	for _, j := range fresh {
		names = append(names, j.Company)
	}
	if !searched || !reflect.DeepEqual(names, []string{"A", "B", "C", "D"}) {
		t.Fatalf("got %v", names)
	}
	first := rep.Sources["first"]
	if first.Calls != 3 || first.Fetched != 10 || first.New != 4 || rep.Sources["second"].Calls != 0 {
		t.Fatalf("report: first %+v second %+v", first, rep.Sources["second"])
	}
	if rep.Skipped["on_skip_list"] != 3 || rep.Skipped["duplicate_or_incomplete"] != 3 || rep.SpendMicro != 3000 {
		t.Fatalf("report: %+v", rep)
	}

	// A target the first source can't meet moves on to the next source; a
	// company already seen there is not counted twice.
	rep = &hiringReport{}
	fresh, _ = m.collectHiringJobs(context.Background(), sources(), 50, skipList, rep)
	if len(fresh) != 6 || rep.Sources["second"].New != 2 || rep.Sources["second"].Fetched != 3 {
		t.Fatalf("got %d: %+v", len(fresh), rep.Sources["second"])
	}

	// The search budget ends the run's pulling, whatever the target.
	rep = &hiringReport{SpendMicro: maxHiringSourceSpendMicro}
	if fresh, searched := m.collectHiringJobs(context.Background(), sources(), 50, skipList, rep); len(fresh) != 0 || searched {
		t.Fatalf("over budget: %v %v", fresh, searched)
	}

	// Every call failing is "not searched": the scan retries in an hour.
	failing := jobSource{name: "down", maxPages: 2, next: func(context.Context, int) (jobPage, error) {
		return jobPage{more: true}, treg.ErrPaused
	}}
	rep = &hiringReport{}
	if _, searched := m.collectHiringJobs(context.Background(), []jobSource{failing}, 5, skipList, rep); searched || rep.Sources["down"].FailedCalls != 2 {
		t.Fatalf("searched=%v report=%+v", searched, rep.Sources["down"])
	}
}

func TestHiringSourceRecordedOnLead(t *testing.T) {
	var meta map[string]any
	if err := json.Unmarshal(hiringMetadata(jobPost{Company: "Acme", Source: sourceBlitz}, "CTO", contactCheck{confirmed: true}, nil), &meta); err != nil || meta["source"] != sourceBlitz {
		t.Fatalf("mention metadata: %v %v", meta, err)
	}
	lead := leadMetadata(companyLead{source: sourceHN, contact: &enrich.Result{}})
	if lead["source"] != sourceHN {
		t.Fatalf("lead metadata: %v", lead)
	}
	if _, ok := leadMetadata(companyLead{contact: &enrich.Result{}})["source"]; ok {
		t.Fatal("a prospect lead has no job source")
	}
}

func TestHiringReportJSON(t *testing.T) {
	rep := &hiringReport{At: time.Date(2026, 11, 15, 6, 0, 0, 0, time.UTC), SpendMicro: 21000, Stopped: "the day's people searches are used up"}
	*rep.source(sourceBlitz) = sourceReport{Calls: 1, Fetched: 20, New: 8, Passed: 5, Stored: 4, Leads: 3}
	rep.skip("screened_out", 3)
	rep.skip("too_big", 0) // nothing skipped: no entry
	want := `{"at":"2026-11-15T06:00:00Z","sources":{"blitz_jobs":{"calls":1,"fetched":20,"new":8,"passed_screen":5,"stored":4,"leads":3}},"skipped":{"screened_out":3},"search_spend_micro":21000,"stopped":"the day's people searches are used up"}`
	if got := string(jsonBytes(rep)); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
