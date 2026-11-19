package monitor

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"

	"leadecho/internal/ai"
	"leadecho/internal/treg"
)

// Job sources for the hiring scan. Each returns postings as jobPost, tagged
// with its name, and all of them feed the one pipeline in scanHiring. The scan
// pulls pages, source by source, until enough new companies are in hand.

const (
	tregBlitzJobsTool = "blitzapi.jobs.search"
	tregHNHiringTool  = "crawl4ai.hackernews.jobs.hiring"

	// In the order they are pulled: the monthly Hacker News thread costs one
	// small call, the filtered search returns companies already in the plan's
	// size range, LinkedIn search returns mostly large employers.
	sourceHN       = "hn_hiring"
	sourceBlitz    = "blitz_jobs"
	sourceLinkedIn = "linkedin_jobs"

	linkedInPagesPerRole = 3 // 10 postings a page
	blitzMaxPages        = 4
	blitzPageMax         = 50 // the API's limit
	blitzPageMin         = 10
	// blitzJobCostMicro is the price of each posting returned ($0.001): a
	// page is sized so its full price stays under treg's per-call ceiling,
	// which refuses a dearer call outright.
	blitzJobCostMicro = 1000
	// New companies sent to the screen per paid people search the plan allows:
	// the screen and the size check drop most of them.
	hiringScreenPerLookup = 3
	// maxHiringSourceSpendMicro stops pulling pages once the job searches of
	// one run have cost this much ($0.15).
	maxHiringSourceSpendMicro = 150_000
	// hiringRecentRun: when the last scan is newer than this, only the last
	// day's postings are asked for; otherwise the last week's.
	hiringRecentRun = 26 * time.Hour
)

// jobPage is one call's worth of postings from a source.
type jobPage struct {
	jobs      []jobPost
	more      bool // the source has another page
	costMicro int64
}

// jobSource pulls one page per call to next. need is how many new companies
// the scan still wants: a source billed per posting asks for twice that (two
// to three postings per company come back), within its page limits.
// After an error the source must still move on, so maxPages bounds the calls.
type jobSource struct {
	name     string
	maxPages int
	next     func(ctx context.Context, need int) (jobPage, error)
}

// hiringSources are the day's sources, in the order they are pulled. since is
// the last completed scan (zero: never).
func (m *Monitor) hiringSources(wsID string, st hiringState, since, now time.Time) []jobSource {
	recent := !since.IsZero() && now.Sub(since) < hiringRecentRun
	if since.IsZero() || now.Sub(since) > maxMentionAge {
		since = now.Add(-maxMentionAge) // older posts would be dropped on insert
	}
	return []jobSource{
		m.hnHiringSource(wsID, st.Plan.Roles, since),
		m.blitzSource(wsID, st.Plan, st.Countries, recent),
		m.linkedInSource(wsID, st.Plan.Roles, recent),
	}
}

// collectHiringJobs pulls pages until target new companies are found, the
// sources run out, or the run's search budget is spent. skipReason says why a
// posting's company is not new ("" when it is). searched is false when no
// call succeeded at all.
func (m *Monitor) collectHiringJobs(ctx context.Context, sources []jobSource, target int, skipReason func(jobPost) string, rep *hiringReport) (fresh []jobPost, searched bool) {
	seen := map[string]bool{}
	// Distinct matching postings per company, across pages and sources (the
	// same posting can come up under two role searches).
	openings := map[string]map[string]bool{}
	defer func() {
		for i := range fresh {
			fresh[i].Openings = len(openings[companyKey(fresh[i].Company)])
		}
	}()
	for _, src := range sources {
		sr := rep.source(src.name)
		for page := 0; page < src.maxPages && len(fresh) < target && rep.SpendMicro < maxHiringSourceSpendMicro && ctx.Err() == nil; page++ {
			p, err := src.next(ctx, target-len(fresh))
			sr.Calls++
			if err != nil {
				sr.FailedCalls++
				if !treg.IsOperatorError(err) {
					m.logger.Debug().Err(err).Str("source", src.name).Msg("hiring: job search failed")
				}
			} else {
				searched = true
			}
			rep.SpendMicro += p.costMicro
			sr.Fetched += len(p.jobs)
			for _, j := range p.jobs {
				key := companyKey(j.Company)
				if j.Title == "" || key == "" {
					continue
				}
				if openings[key] == nil {
					openings[key] = map[string]bool{}
				}
				openings[key][strings.ToLower(strings.TrimSpace(j.Title))+"|"+j.Location] = true
			}
			unique := onePerCompany(p.jobs)
			rep.skip("duplicate_or_incomplete", len(p.jobs)-len(unique))
			for _, job := range unique {
				key := companyKey(job.Company)
				reason := "duplicate_or_incomplete" // the company came up on an earlier page
				if !seen[key] {
					reason = skipReason(job)
				}
				seen[key] = true
				if reason != "" {
					rep.skip(reason, 1)
					continue
				}
				if len(fresh) < maxHiringScreened {
					sr.New++
					fresh = append(fresh, job)
				}
			}
			if !p.more {
				break
			}
		}
	}
	return fresh, searched
}

// linkedInSource pages each role's search: every role's first page before any
// role's second, since a search's first page is its best.
func (m *Monitor) linkedInSource(wsID string, roles []string, recent bool) jobSource {
	posted := "week"
	if recent {
		posted = "24h"
	}
	cursors, done, turn := make([]string, len(roles)), make([]bool, len(roles)), 0
	left := func() bool {
		for _, d := range done {
			if !d {
				return true
			}
		}
		return false
	}
	return jobSource{name: sourceLinkedIn, maxPages: len(roles) * linkedInPagesPerRole, next: func(ctx context.Context, _ int) (jobPage, error) {
		if !left() {
			return jobPage{}, nil
		}
		for done[turn%len(roles)] {
			turn++
		}
		r := turn % len(roles)
		turn++
		body := map[string]string{"query": roles[r], "postedLimit": posted}
		if cursors[r] != "" {
			body["cursor"] = cursors[r]
		}
		resp, err := m.treg.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tregJobsTool, Body: body})
		if err != nil {
			done[r] = true
			return jobPage{more: left()}, err
		}
		jobs := tagSource(parseJobs(resp.Body), sourceLinkedIn)
		cursors[r] = jobsNextCursor(resp.Body)
		done[r] = cursors[r] == "" || len(jobs) == 0
		return jobPage{jobs: jobs, more: left(), costMicro: resp.CostMicro}, nil
	}}
}

// jobsNextCursor reads the next-page cursor beside the items parseJobs reads.
func jobsNextCursor(body []byte) string {
	type data struct {
		NextCursor string `json:"nextCursor"`
	}
	var v struct {
		Output *struct {
			Data data `json:"data"`
		} `json:"output"`
		Data data `json:"data"`
	}
	if json.Unmarshal(body, &v) != nil {
		return ""
	}
	if v.Output != nil {
		return v.Output.Data.NextCursor
	}
	return v.Data.NextCursor
}

func tagSource(jobs []jobPost, source string) []jobPost {
	for i := range jobs {
		jobs[i].Source = source
	}
	return jobs
}

// blitzSource searches postings by title with the plan's company size and the
// workspace's countries applied by the provider, agencies excluded.
func (m *Monitor) blitzSource(wsID string, plan *ai.HiringPlan, countries []string, recent bool) jobSource {
	cursor, done := "", false
	pageMax := blitzPageMax
	if ceiling := m.treg.MaxCostMicro(); ceiling > 0 {
		pageMax = min(pageMax, int(ceiling/blitzJobCostMicro))
	}
	if pageMax < 1 {
		return jobSource{name: sourceBlitz} // the ceiling is under one posting's price
	}
	return jobSource{name: sourceBlitz, maxPages: blitzMaxPages, next: func(ctx context.Context, need int) (jobPage, error) {
		if done {
			return jobPage{}, nil
		}
		resp, err := m.treg.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tregBlitzJobsTool,
			Body: blitzJobsBody(plan, countries, recent, min(max(2*need, blitzPageMin), pageMax), cursor)})
		if err != nil {
			done = true
			return jobPage{}, err
		}
		jobs, next := parseBlitzJobs(resp.Body, plan.MaxCompanySize > 0)
		cursor, done = next, next == "" || len(jobs) == 0
		return jobPage{jobs: jobs, more: !done, costMicro: resp.CostMicro}, nil
	}}
}

// blitzJobsBody is the search request. No location filter is sent when the
// workspace lists no countries.
func blitzJobsBody(plan *ai.HiringPlan, countries []string, recent bool, maxResults int, cursor string) map[string]any {
	days := 7
	if recent {
		days = 2 // a day of overlap: postings reach the index late
	}
	job := map[string]any{
		"title":       map[string]any{"include": plan.Roles},
		"date_posted": map[string]any{"last_days": days},
	}
	if cc := countryCodes(countries); len(cc) > 0 {
		job["location"] = map[string]any{"country_code": map[string]any{"include": cc}}
	}
	company := map[string]any{"is_agency": false}
	if plan.MaxCompanySize > 0 {
		company["employee_count"] = map[string]any{"min": plan.MinSize(), "max": plan.MaxCompanySize}
	}
	body := map[string]any{"job": job, "company": company, "max_results": maxResults}
	if cursor != "" {
		body["cursor"] = cursor
	}
	return body
}

// countryCodes keeps the two-letter codes of a hand-edited setting, upper-cased.
func countryCodes(in []string) []string {
	var out []string
	for _, c := range in {
		if c = strings.ToUpper(strings.TrimSpace(c)); len(c) == 2 && c[0] >= 'A' && c[0] <= 'Z' && c[1] >= 'A' && c[1] <= 'Z' {
			out = append(out, c)
		}
	}
	return out
}

// parseBlitzJobs reads {results:[...],cursor} (or the same under "output").
// sizeFiltered says the request carried the plan's size range, so every
// company returned is within it.
func parseBlitzJobs(body []byte, sizeFiltered bool) (jobs []jobPost, cursor string) {
	type page struct {
		Results []struct {
			DatePosted  string `json:"date_posted"`
			Title       string `json:"title"`
			URL         string `json:"url"`
			CompanyName string `json:"company_name"`
			CompanyURL  string `json:"company_linkedin_url"`
			Location    struct {
				City    string `json:"city"`
				Country string `json:"country_code"`
			} `json:"location"`
		} `json:"results"`
		Cursor string `json:"cursor"`
	}
	var v struct {
		Output *page `json:"output"`
		page
	}
	if json.Unmarshal(body, &v) != nil {
		return nil, ""
	}
	got := v.page
	if v.Output != nil {
		got = *v.Output
	}
	for _, r := range got.Results {
		job := jobPost{Company: r.CompanyName, CompanyURL: r.CompanyURL, Title: r.Title, URL: r.URL,
			Location: joinNonEmpty(", ", r.Location.City, r.Location.Country), Country: r.Location.Country, Source: sourceBlitz, SizeFits: sizeFiltered}
		// "2026-11-15 04:27:04+01", sometimes with fractional seconds (which
		// time.Parse accepts without the layout naming them).
		if t, err := time.Parse("2006-01-02 15:04:05-07", r.DatePosted); err == nil {
			job.CreatedUTC = t.Unix()
		}
		jobs = append(jobs, job)
	}
	return jobs, got.Cursor
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

// hnHiringSource reads this month's "Who is hiring?" thread, once per distinct
// role keyword, keeping only posts made since the last scan.
func (m *Monitor) hnHiringSource(wsID string, roles []string, since time.Time) jobSource {
	kws, i := roleKeywords(roles), 0
	return jobSource{name: sourceHN, maxPages: len(kws), next: func(ctx context.Context, _ int) (jobPage, error) {
		if i >= len(kws) {
			return jobPage{}, nil
		}
		kw := kws[i]
		i++
		resp, err := m.treg.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tregHNHiringTool,
			Body: map[string]any{"keyword": kw, "remote": false}})
		if err != nil {
			return jobPage{more: i < len(kws)}, err
		}
		return jobPage{jobs: parseHNHiring(resp.Body, kw, kws, since), more: i < len(kws), costMicro: resp.CostMicro}, nil
	}}
}

// roleKeywords is the head noun of each role ("senior backend engineer" ->
// "engineer"): the thread is searched by one word.
func roleKeywords(roles []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range roles {
		f := strings.Fields(strings.ToLower(r))
		if len(f) == 0 {
			continue
		}
		if kw := nonAlnumRe.ReplaceAllString(f[len(f)-1], ""); len(kw) >= 3 && !seen[kw] {
			seen[kw] = true
			out = append(out, kw)
		}
	}
	return out
}

const maxHNField = 120 // runes kept of a title or location taken from a post

// parseHNHiring reads {rows:[{company,posted,text,url}]} (or the same under
// "output"). A post's first line is "Company | Role | Location | ..." by
// convention only: the company can be wrong (a city, a tagline), and the
// model screen is what rejects those. The role is the first field naming one
// of the keywords. When none does (the roles are further down the post), it
// is kw, the word the thread was searched by, which the post does contain.
func parseHNHiring(body []byte, kw string, keywords []string, since time.Time) []jobPost {
	type page struct {
		Rows []struct {
			Company string `json:"company"`
			Posted  string `json:"posted"`
			Text    string `json:"text"`
			URL     string `json:"url"`
		} `json:"rows"`
	}
	var v struct {
		Output *page `json:"output"`
		page
	}
	if json.Unmarshal(body, &v) != nil {
		return nil
	}
	got := v.page
	if v.Output != nil {
		got = *v.Output
	}
	var jobs []jobPost
	for _, r := range got.Rows {
		posted, err := time.Parse(time.RFC3339, r.Posted)
		if err != nil || !posted.After(since) {
			continue
		}
		header, _, _ := strings.Cut(strings.TrimSpace(r.Text), "\n")
		var fields []string
		for _, f := range strings.Split(header, "|") {
			if f = strings.TrimSpace(f); f != "" && !strings.Contains(f, "://") {
				fields = append(fields, f)
			}
		}
		if len(fields) < 2 {
			continue // no role to screen
		}
		company := r.Company
		if company == "" {
			company = fields[0]
		}
		company = strings.Trim(strings.TrimSpace(parenRe.ReplaceAllString(company, " ")), "*") // "MixRank (YC S11)" -> "MixRank"
		if company == "" || len([]rune(company)) > 60 {
			continue // a sentence, not a name
		}
		title, rest := kw, fields[1:]
		for i := 1; i < len(fields); i++ {
			if containsAnyFold(fields[i], keywords) {
				title, rest = fields[i], append(append([]string{}, fields[1:i]...), fields[i+1:]...)
				break
			}
		}
		jobs = append(jobs, jobPost{
			ID: r.URL, Company: company, CompanyURL: companySiteURL(r.Text), Title: clipRunes(title, maxHNField),
			Location: clipRunes(strings.Join(rest, ", "), maxHNField), URL: r.URL, CreatedUTC: posted.Unix(), Source: sourceHN,
		})
	}
	return jobs
}

func containsAnyFold(s string, words []string) bool {
	s = strings.ToLower(s)
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func clipRunes(s string, n int) string {
	if rs := []rune(s); len(rs) > n {
		return string(rs[:n])
	}
	return s
}

var (
	textURLRe = regexp.MustCompile(`https?://[^\s<>"'()\[\]]+`)
	// jobBoardHosts are where postings link to that is not the company's own
	// site: applicant-tracking systems, form and document hosts.
	// ponytail: a short list; a link to an unlisted board just leaves the
	// contact unconfirmed (its employer's website will not match).
	jobBoardHosts = map[string]bool{
		"ashbyhq.com": true, "greenhouse.io": true, "lever.co": true, "workable.com": true, "linkedin.com": true,
		"ycombinator.com": true, "workatastartup.com": true, "wellfound.com": true, "notion.site": true, "notion.so": true,
		"google.com": true, "forms.gle": true, "github.com": true, "gem.com": true, "breezy.hr": true, "bamboohr.com": true,
		"rippling.com": true, "smartrecruiters.com": true, "myworkdayjobs.com": true, "jobvite.com": true, "recruitee.com": true,
		"teamtailor.com": true, "personio.com": true, "personio.de": true, "airtable.com": true, "typeform.com": true,
		"bit.ly": true, "youtube.com": true, "x.com": true, "twitter.com": true, "medium.com": true, "substack.com": true,
		"calendly.com": true, "apple.com": true, "dover.com": true, "pinpointhq.com": true, "icims.com": true,
	}
)

// companySiteURL is the first link in a post that is not a job board, cut
// down to the site ("https://acme.io"); "" when there is none.
func companySiteURL(text string) string {
	for _, raw := range textURLRe.FindAllString(text, -1) {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			continue
		}
		host := strings.ToLower(u.Hostname())
		reg, err := publicsuffix.EffectiveTLDPlusOne(host)
		if err != nil || jobBoardHosts[reg] {
			continue
		}
		return u.Scheme + "://" + host
	}
	return ""
}

// jobSiteDomain is the company website the entry itself names: a Hacker News
// post links to it, a funding round comes with it. Other postings carry none.
func jobSiteDomain(job jobPost) string {
	if job.Source != sourceHN && job.Source != sourceFunding {
		return ""
	}
	return cleanDomain(job.CompanyURL)
}

// sourceReport counts one source's postings through a run.
type sourceReport struct {
	Calls       int `json:"calls"`
	FailedCalls int `json:"failed_calls,omitempty"`
	Fetched     int `json:"fetched"`       // postings returned
	New         int `json:"new"`           // new companies sent to the screen
	Passed      int `json:"passed_screen"` // kept by the screen
	Stored      int `json:"stored"`        // signals stored, confirmed or not
	Leads       int `json:"leads"`         // stored with a confirmed contact
}

// hiringReport is what the last run did, kept under
// workspaces.settings.hiring_signals.last_report so an operator can see why a
// day produced few leads.
type hiringReport struct {
	At      time.Time                `json:"at"`
	Sources map[string]*sourceReport `json:"sources"`
	// Skipped counts postings by the reason they did not become a lead.
	Skipped    map[string]int `json:"skipped,omitempty"`
	SpendMicro int64          `json:"search_spend_micro"` // what the job searches cost, micro-USD
	// FundingSpendMicro is what the day's funding rounds cost (sources.funding
	// counts them: fetched = rounds billed, new = companies sent to the screen).
	FundingSpendMicro int64  `json:"funding_spend_micro,omitempty"`
	Stopped           string `json:"stopped,omitempty"` // why the run ended before every company was tried
	// Retry: the run could not do its work and the claim was moved, so the
	// next one is due in an hour instead of tomorrow.
	Retry bool `json:"retry,omitempty"`
	// CarriedIn: screened companies an earlier run did not reach, tried first
	// by this one. CarriedOver: those this run leaves for the next.
	CarriedIn   int `json:"carried_in,omitempty"`
	CarriedOver int `json:"carried_over,omitempty"`
}

func (r *hiringReport) source(name string) *sourceReport {
	if r.Sources == nil {
		r.Sources = map[string]*sourceReport{}
	}
	if r.Sources[name] == nil {
		r.Sources[name] = &sourceReport{}
	}
	return r.Sources[name]
}

func (r *hiringReport) skip(reason string, n int) {
	if n <= 0 {
		return
	}
	if r.Skipped == nil {
		r.Skipped = map[string]int{}
	}
	r.Skipped[reason] += n
}
