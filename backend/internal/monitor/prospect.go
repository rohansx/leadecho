package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/net/publicsuffix"

	"leadecho/internal/ai"
	"leadecho/internal/billing"
	"leadecho/internal/database"
	"leadecho/internal/enrich"
	"leadecho/internal/treg"
)

// Prospecting: companies that fit one of the workspace's customer segments,
// found by company search instead of by something they posted. Each run
// takes one segment, searches for companies in it, has the model drop the
// ones that only match by keyword, and then treats each fitting company like
// a hiring signal: find the buyer, confirm them, research the company and
// draft a first message. Only confirmed contacts become leads.

const (
	// Exa, not the routed treg.companies.search: the router's first hit
	// (Hunter) returns only name and domain and ignores the country, which
	// leaves the fit check nothing to judge. Exa returns a description,
	// headcount and HQ, reads the location from the query, and costs about
	// $0.012 for 15 rows.
	tregCompanySearchTool  = "exa.companies.search"
	maxProspectDescription = 500 // runes of a company description sent to the fit check

	prospectSettingsKey = "prospecting"
	prospectInterval    = 24 * time.Hour
	// Rows per company search: the provider bills per row, so this is the
	// price dial. Most rows are dropped by the fit check.
	prospectSearchLimit = 25 // Exa bills per result, ~$0.001: more rows, fewer dry runs
	// People searches per run (each then pays for a contact lookup).
	maxProspectContactsPerRun = 5 // Pro
	starterProspectContacts   = 2 // Starter
	maxProspectSeen           = 2000
	maxProspectName           = 120

	// A prospect has no event behind it, so it ranks under a hiring signal
	// with the same contact quality.
	prospectScoreDiscount = 0.5
)

// ProspectJudge checks a company search's results against a segment.
type ProspectJudge interface {
	FitProspects(ctx context.Context, workspaceID string, in ai.ProspectCheck) ([]ai.ProspectFit, error)
}

// SetProspectJudge turns prospecting on. It also needs the hiring planner,
// whose research and drafting it shares.
func (m *Monitor) SetProspectJudge(j ProspectJudge) { m.prospectJudge = j }

// prospectState is stored under workspaces.settings.prospecting.
type prospectState struct {
	LastRun time.Time `json:"last_run"`
	// Round counts runs: it picks the segment, and once every segment has
	// had a turn, the next wording of each segment's search.
	Round int `json:"round"`
	// Seen holds company domains already judged, so a company is never
	// checked (or contacted) twice.
	Seen []string `json:"seen,omitempty"`
	// Last is what happened to each company in the latest run, so an
	// operator can tell a strict filter from a broken step.
	Last []prospectOutcome `json:"last,omitempty"`
	// Found holds, per segment name, the companies that became leads, oldest
	// first and only appended to: later searches look for companies like
	// them (see segmentQueries).
	Found map[string][]string `json:"found,omitempty"`
	// Next is, per segment name, which of its search wordings runs next. A
	// cursor, not Round: the list of wordings grows as leads are found.
	Next map[string]int `json:"next,omitempty"`
}

const maxProspectFound = 10 // per segment

type prospectOutcome struct {
	Company string `json:"company"`
	Outcome string `json:"outcome"`
}

// prospectClaimInterval is a day, or at once for an operator's manual scan.
func prospectClaimInterval(ctx context.Context) time.Duration {
	if forcedScan(ctx) == ScanProspects {
		return 0
	}
	return prospectInterval
}

// prospectBlocked is why prospecting can't run for the workspace, "" if it can.
func (m *Monitor) prospectBlocked(ctx context.Context, ws string) string {
	if m.prospectJudge == nil {
		return "prospecting is not set up on this server"
	}
	return m.hiringBlocked(ctx, ws) // same needs: paid data, the planner, a profile
}

// prospectableSegments returns the profile's segments that are detailed
// enough to search for (profiles from before segments had criteria are not).
func prospectableSegments(bp database.BrandProfile) []ai.CustomerSegment {
	var all, out []ai.CustomerSegment
	if json.Unmarshal(bp.CustomerSegments, &all) != nil {
		return nil
	}
	for _, s := range all {
		if s.Prospectable() {
			out = append(out, s)
		}
	}
	return out
}

// prospectSegments loads the workspace's prospectable segments.
func (m *Monitor) prospectSegments(ctx context.Context, ws string) []ai.CustomerSegment {
	bp, err := m.q.GetBrandProfile(ctx, ws)
	if err != nil {
		return nil
	}
	return prospectableSegments(bp)
}

// prospectDailyOn reports whether the scheduled scan runs: Pro only, and on
// unless an admin switched it off.
func (m *Monitor) prospectDailyOn(ctx context.Context, wsID string) bool {
	bs, err := m.q.GetBillingState(ctx, wsID)
	if err != nil {
		return false
	}
	stored, err := m.q.GetWorkspaceSettingsKey(ctx, wsID, billing.ProspectDailyKey)
	return err == nil && billing.ProspectDaily(bs.Plan, m.credits.Enforced(), stored)
}

func (m *Monitor) scanProspects(ctx context.Context, wsID string) {
	if m.prospectJudge == nil || m.hiringPlanner == nil || !m.treg.Enabled() || m.credits.PaidPlatformsLocked(ctx, wsID) || m.credits.PaidDataPaused(ctx) {
		return
	}
	if forcedScan(ctx) != ScanProspects && !m.prospectDailyOn(ctx, wsID) {
		return // the daily run is Pro's, and an admin can switch it off
	}
	bp, err := m.q.GetBrandProfile(ctx, wsID)
	if err != nil || bp.Product == "" {
		return
	}
	segments := prospectableSegments(bp)
	if len(segments) == 0 {
		return // nothing to search for: checked before claiming, so it costs no run
	}
	claimed, err := m.q.ClaimSettingsRun(ctx, wsID, prospectSettingsKey, prospectClaimInterval(ctx))
	if err != nil || !claimed {
		if err != nil {
			m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("prospects: claim failed")
		}
		return
	}
	st, ok := m.loadProspectState(ctx, wsID)
	if !ok {
		return
	}
	seg := segments[st.Round%len(segments)]
	queries := segmentQueries(seg, st.Found[seg.Name])
	if st.Next == nil {
		st.Next = map[string]int{}
	}
	next, ok := st.Next[seg.Name]
	if !ok {
		next = st.Round / len(segments) // state from before the cursor: carry on where it was
	}
	query := queries[next%len(queries)]
	st.Next[seg.Name] = (next + 1) % len(queries)
	st.Round++
	defer func() { // outlive a shutdown: losing Seen means paying to judge the same companies again
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		m.saveProspectState(saveCtx, wsID, st)
	}()

	seen := map[string]bool{}
	for _, d := range st.Seen {
		seen[d] = true
	}
	remember := func(domain string) {
		if seen[domain] {
			return
		}
		seen[domain] = true
		// Full: forget the oldest, so new companies are still never paid for twice.
		if st.Seen = append(st.Seen, domain); len(st.Seen) > maxProspectSeen {
			st.Seen = st.Seen[len(st.Seen)-maxProspectSeen:]
		}
	}
	st.Last = nil
	note := func(company, outcome string) {
		// Clipped here: outcomes carry provider and profile text.
		st.Last = append(st.Last, prospectOutcome{Company: truncateRunes(company, maxProspectName), Outcome: truncateRunes(outcome, 240)})
	}
	candidates, err := m.searchCompanies(ctx, wsID, query, seg.Country)
	if err != nil {
		note("", "company search failed: "+err.Error())
		return // out of credits, paused or provider down: the round still advances
	}
	candidates = newProspects(candidates, seen, bp.SourceUrl, bp.Competitors)
	if len(candidates) == 0 {
		m.logger.Info().Str("segment", seg.Name).Str("workspace_id", wsID).Msg("prospects: no new companies")
		note("", "the search found no companies not already judged")
		return
	}
	fits, err := m.prospectJudge.FitProspects(ctx, wsID, ai.ProspectCheck{
		Product: bp.Product, Overview: bp.Overview, Segment: seg, Competitors: bp.Competitors, Candidates: candidates,
	})
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("prospects: fit check failed")
		note("", "fit check failed: "+err.Error())
		return // the companies stay unseen: judged on a later run
	}
	fitting := map[int]bool{}
	for _, f := range fits {
		fitting[f.Index] = true
	}
	for i, c := range candidates {
		if !fitting[i] {
			remember(c.Domain) // judged not to fit: never paid for again
			note(c.Name, "not a fit for the segment")
		}
	}
	// A fitting company is remembered once its people search has run, so
	// one left over by today's cap or a failed search is found again later.

	capContacts := starterProspectContacts
	if bs, err := m.q.GetBillingState(ctx, wsID); err == nil && bs.Plan == "pro" {
		capContacts = maxProspectContactsPerRun
	}
	created, lookups := 0, 0
	for i, fit := range fits {
		if lookups >= capContacts || ctx.Err() != nil {
			for _, rest := range fits[i:] {
				note(candidates[rest.Index].Name, "fits; left for a later run (daily contact cap)")
			}
			break
		}
		c := candidates[fit.Index]
		pid := prospectPlatformID(c.Domain)
		exists, err := m.q.MentionExists(ctx, wsID, string(database.PlatformTypeExa), pid)
		if err != nil {
			note(c.Name, "skipped: database error")
			continue
		}
		if exists {
			remember(c.Domain)
			note(c.Name, "already a lead")
			continue
		}
		lookups++
		done, stop, outcome := m.prospectCompany(ctx, wsID, bp, seg, c, fit, pid, remember)
		note(c.Name, outcome)
		if done {
			created++
			if st.Found == nil {
				st.Found = map[string][]string{}
			}
			// Appended, so the wordings already run keep their places.
			// ponytail: the first 10 finds seed searches; later ones do not.
			if f := st.Found[seg.Name]; len(f) < maxProspectFound {
				st.Found[seg.Name] = append(f, c.Name)
			}
		}
		if stop {
			break
		}
	}
	m.logger.Info().Str("segment", seg.Name).Int("searched", len(candidates)).Int("fit", len(fits)).
		Int("people_searches", lookups).Int("leads", created).Str("workspace_id", wsID).Msg("prospects: scan done")
}

// prospectCompany finds and confirms the buyer at one fitting company and
// stores the lead. stop reports that the run should end (a failed search,
// the daily contact cap, or shutdown). remember is called once the paid
// people search has run.
// outcome says what happened, in plain words, for the run's record.
func (m *Monitor) prospectCompany(ctx context.Context, wsID string, bp database.BrandProfile, seg ai.CustomerSegment, c ai.ProspectCandidate, fit ai.ProspectFit, pid string, remember func(string)) (created, stop bool, outcome string) {
	job := jobPost{Company: c.Name, CompanyURL: "https://" + c.Domain} // no job: the buyer query names the titles
	candidates, err := m.searchBuyers(ctx, wsID, seg.BuyerTitles, job, c.Employees,
		func(p []foundPerson) []foundPerson { return rankByTitles(p, seg.BuyerTitles) })
	if err != nil {
		return false, true, "people search failed: " + err.Error()
	}
	// Titles from the search are free to check: a person whose title does
	// not buy is never paid for. The looked-up title is checked again.
	candidates = slices.DeleteFunc(candidates, func(p foundPerson) bool {
		return p.Title != "" && !titleOwnsBuying(p.Title, seg.BuyerTitles, c.Employees)
	})
	if len(candidates) == 0 {
		remember(c.Domain)
		return false, false, "no one with a buyer title (" + truncateRunes(seg.BuyerTitles, 80) + ") found"
	}
	co := hiringCompany{size: c.Employees, domain: c.Domain}
	// Up to maxContactTries paid lookups per company: a catch-all email sends
	// it to the next person, so a run pays for at most 2 x its contact cap.
	person, contact, check, capped := chooseContact(candidates,
		func(p foundPerson) (*enrich.Result, bool) { return m.verifyContact(ctx, wsID, job, p) },
		func(p foundPerson, res *enrich.Result) contactCheck {
			return confirmProspectContact(c.Name, co, p, res, seg.BuyerTitles)
		})
	if capped && !check.confirmed {
		return false, true, "daily contact lookups used up; tried again on a later run"
	}
	remember(c.Domain)
	if !check.confirmed {
		// Without a confirmed buyer a fitting company is only a name on a
		// list: not stored, and (already judged) not looked up again.
		// What disagreed goes first (outcomes are clipped): it tells a
		// contact who moved on from a brand name the check does not know.
		if check.mismatch != "" {
			return false, capped, "no buyer confirmed [" + check.mismatch + "]: " + check.reason
		}
		return false, capped, "no buyer confirmed: " + check.reason
	}
	brief, researchErr := m.researchLeadErr(ctx, wsID, ai.CompanyResearch{
		Product: bp.Product, Overview: bp.Overview, Audience: bp.TargetAudience,
		Company: c.Name, CompanyURL: job.CompanyURL, ContactTitle: person.Title,
		Fit: seg.Name + ": " + strings.Join(seg.Criteria, "; "), Employees: c.Employees,
	})
	if ctx.Err() != nil {
		m.refundResearch(ctx, wsID, brief)
		return false, true, "stopped: shutting down"
	}
	if brief != nil && brief.NotABuyer != "" {
		// Its own pages contradict the fit (too small, not hiring, a vendor):
		// not stored, not charged, not looked at again.
		m.logger.Info().Str("company", c.Name).Str("why", brief.NotABuyer).Str("workspace_id", wsID).Msg("prospects: research ruled the company out")
		m.refundResearch(ctx, wsID, brief)
		return false, capped, "ruled out by research: " + truncateRunes(brief.NotABuyer, 200)
	}
	if c.Employees > 0 {
		// The company search's headcount, not the contact provider's: the
		// latter was wrong live (BillDesk 11, Perfios 54).
		fixed := *contact
		fixed.CompanySize = c.Employees
		fixed.Employees = strconv.Itoa(c.Employees) // the stored lead showed both (Juspay 2071 and 1316)
		contact = &fixed
	}
	mentionID := m.insertProspect(ctx, wsID, pid, seg, c, fit, person, contact, brief)
	if mentionID == "" {
		m.refundResearch(ctx, wsID, brief)
		return false, capped, "not stored (duplicate or muted)"
	}
	if brief == nil {
		// No sourced fact to open on: a draft would be the same generic
		// question for everyone, so none is written.
		why := "research found nothing"
		if researchErr != nil {
			why = "research failed: " + truncateRunes(researchErr.Error(), 200)
		}
		return true, capped, "lead without a draft (" + why + ")"
	}
	m.draftOutreach(ctx, wsID, mentionID, ai.Outreach{
		Product: bp.Product, Overview: bp.Overview, Positioning: bp.Positioning,
		ContactName: person.Name, ContactTitle: person.Title, Company: c.Name,
		Research: brief, SegmentPain: seg.Pain, CompanySize: c.Employees,
	})
	return true, capped, "lead"
}

// segmentQueries are the wordings of a segment's company search, one per
// run in turn. They describe the buyer's company (the criteria), never the
// segment's name: a name like "Technical recruiters" made the search return
// recruiting firms (live, 2026-11-15). The same query returns the same
// companies, all seen after the first run, so later runs add one company at
// a time for the search to find others like it: the segment's examples,
// then the companies that became leads (found, oldest first; new ones are
// added at the end, so the cursor in prospectState.Next reaches them).
// Without them the search ran dry (live: 3 new companies in a run).
func segmentQueries(s ai.CustomerSegment, found []string) []string {
	base := strings.Join(s.Criteria, "; ")
	out := []string{base}
	for _, ex := range append(append([]string(nil), s.ExampleCompanies...), found...) {
		if q := base + "; similar to " + ex; !slices.Contains(out, q) {
			out = append(out, q)
		}
	}
	return out
}

func prospectPlatformID(domain string) string { return "prospect-" + domain }

// searchCompanies runs one company search. An error means the search
// itself failed; no companies with a nil error means it found none.
func (m *Monitor) searchCompanies(ctx context.Context, wsID, query, country string) ([]ai.ProspectCandidate, error) {
	if country != "" {
		query += ", based in " + countryName(country)
	}
	body := map[string]any{"query": query, "category": "company", "numResults": prospectSearchLimit}
	resp, err := m.treg.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tregCompanySearchTool, Body: body})
	if err != nil {
		if !treg.IsOperatorError(err) {
			m.logger.Debug().Err(err).Str("workspace_id", wsID).Msg("prospects: company search failed")
		}
		return nil, err
	}
	return parseCompanies(resp.Body), nil
}

// confirmProspectContact is confirmContact, but a catch-all email does not
// count: with no event behind the lead, a message that may bounce is not
// worth sending, so the next candidate is tried instead. Nor does a title
// that does not own the segment's buyer decision (see titleOwnsBuying).
func confirmProspectContact(company string, co hiringCompany, p foundPerson, res *enrich.Result, buyerTitles string) contactCheck {
	ck := confirmContact(company, co, 0, p, res)
	if ck.confirmed && res.EmailStatus == "catchall" {
		return contactCheck{reason: "could not verify " + p.Name + "'s email (the company's mail server accepts any address)"}
	}
	title := p.Title
	if res != nil && res.Title != "" {
		title = res.Title
	}
	if ck.confirmed && !titleOwnsBuying(title, buyerTitles, co.size) {
		return contactCheck{reason: p.Name + "'s title (" + truncateRunes(title, 80) + ") is not one that buys this"}
	}
	return ck
}

// titleAliases spell out the long forms of titles so they match the short
// keys buyer titles use ("Chief Technology Officer" is a CTO).
var titleAliases = strings.NewReplacer(
	"chief technology officer", "cto", "chief technical officer", "cto",
	"chief product and technology officer", "cto cpto", "chief product & technology officer", "cto cpto",
	"chief executive officer", "ceo", "vice president", "vp",
	"human resources", "hr",
)

// hiringWords widen a buyer title about recruiting to the other names that
// role goes by: "Head of HR" owns hiring where "Talent Acquisition" is asked for.
var hiringWords = []string{"talent", "acquisition", "recruiting", "recruitment", "recruiter", "hiring", "hr", "people"}

// sellingWords mark a title on the selling side of a company: "VP & Head of
// Business Engineering solutions" has Engineering in it but sells (live,
// Cashfree 2026-11-15). Ignored when the buyer titles ask for that side.
var sellingWords = []string{"sales", "presales", "account", "accounts", "solution", "solutions", "client", "revenue", "partnerships"}

// titleOwnsBuying reports whether a contact's title matches one of the
// segment's buyer titles, and is not a selling-side title. At a company
// under 200 people, or of unknown size, a founder or CEO owns every buying
// decision (AlgoTest, live: no CTO or engineering head among those found).
// ponytail: word lists; use the model to judge titles if these misfire.
func titleOwnsBuying(title, buyerTitles string, employees int) bool {
	words := titleWords(title)
	asked := titleWords(buyerTitles)
	for _, w := range sellingWords {
		if words[w] && !asked[w] {
			return false
		}
	}
	keys := titleKeys(buyerTitles)
	for _, k := range keys {
		if slices.Contains(hiringWords, k) {
			keys = append(keys, hiringWords...)
			break
		}
	}
	if employees < 200 {
		keys = append(keys, "founder", "cofounder", "ceo")
	}
	for _, k := range keys {
		if words[k] {
			return true
		}
	}
	return false
}

func titleWords(s string) map[string]bool {
	words := map[string]bool{}
	for _, w := range nonWordRe.Split(titleAliases.Replace(strings.ToLower(s)), -1) {
		words[w] = true
	}
	return words
}

// titleKeys are the topic words of the buyer titles, in order: "CTO and VP
// Engineering & Product" gives cto, engineering, product. Rank words (VP,
// Head, Lead, Manager) are dropped: on their own they match any department.
func titleKeys(titles string) []string {
	var keys []string
	for _, w := range nonWordRe.Split(titleAliases.Replace(strings.ToLower(titles)), -1) {
		if w != "" && !rankWords[w] && !slices.Contains(keys, w) {
			keys = append(keys, w)
		}
	}
	return keys
}

var rankWords = map[string]bool{
	"vp": true, "svp": true, "evp": true, "avp": true, "head": true, "of": true, "director": true, "lead": true,
	"leader": true, "manager": true, "senior": true, "sr": true, "chief": true, "officer": true, "the": true,
	"and": true, "or": true, "president": true, "principal": true, "global": true, "group": true, "deputy": true,
	"associate": true, "assistant": true, "for": true, "in": true, "at": true, "a": true,
}

// rankByTitles puts the people whose title matches an earlier buyer title
// first: "VP Engineering or Head of Talent" tries engineering leaders before
// recruiters. A phrase matches on its last word (Engineering, Talent, CTO).
// ponytail: last-word match; use the model to rank if titles get subtler.
var (
	titleSplitRe = regexp.MustCompile(`(?i)\s+or\s+|,|;|/`)
	nonWordRe    = regexp.MustCompile(`[^a-z0-9]+`)
)

func rankByTitles(people []foundPerson, titles string) []foundPerson {
	keys := titleKeys(titles)
	rank := func(title string) int {
		words := titleWords(title)
		for i, k := range keys {
			if words[k] { // whole words: "cto" must not match "director"
				return i
			}
		}
		return len(keys)
	}
	matched := func(p foundPerson) bool { return rank(p.Title) < len(keys) }
	out := append([]foundPerson(nil), people...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if matched(a) != matched(b) {
			return matched(a)
		}
		// Among buyers, the one who owns the decision first: a Head of
		// Talent Acquisition before a Senior HR Manager (live, Juspay).
		if sa, sb := titleSeniority(a.Title), titleSeniority(b.Title); sa != sb {
			return sa > sb
		}
		return rank(a.Title) < rank(b.Title)
	})
	return out
}

// titleSeniority: 3 owns a function (chief, VP, head, director, founder), 2
// leads a team, 1 manages, 0 anything else. An HR or People title with no
// recruiting word in it is a generalist and drops a step.
func titleSeniority(title string) int {
	w := titleWords(title)
	n := 0
	switch {
	case w["chief"] || w["cto"] || w["ceo"] || w["vp"] || w["svp"] || w["evp"] || w["head"] || w["director"] || w["founder"] || w["cofounder"]:
		n = 3
	case w["lead"] || w["principal"]:
		n = 2
	case w["manager"]:
		n = 1
	}
	if (w["hr"] || w["people"]) && !w["talent"] && !w["acquisition"] && !w["recruiting"] && !w["recruitment"] && !w["hiring"] {
		n--
	}
	return n
}

// countryName turns an ISO code into the words a search query understands.
// ponytail: the markets customers name so far; an unlisted code is sent as is.
func countryName(code string) string {
	names := map[string]string{
		"IN": "India", "US": "the United States", "GB": "the United Kingdom", "CA": "Canada",
		"AU": "Australia", "DE": "Germany", "FR": "France", "NL": "the Netherlands",
		"SG": "Singapore", "AE": "the United Arab Emirates", "IE": "Ireland", "ES": "Spain",
	}
	if n, ok := names[strings.ToUpper(code)]; ok {
		return n
	}
	return code
}

// parseCompanies reads Exa's {results:[{url, title, entities:[{properties}]}]}
// or a routed {output:{companies:[...]}} (or unwrapped). Rows are
// provider-native, so each field is taken from whichever key the provider
// uses. A row without a usable website domain is dropped: the domain is
// what the contact's employer is checked against.
func parseCompanies(body []byte) []ai.ProspectCandidate {
	var v struct {
		Output *struct {
			Companies []map[string]any `json:"companies"`
			Results   []map[string]any `json:"results"`
		} `json:"output"`
		Companies []map[string]any `json:"companies"`
		Results   []map[string]any `json:"results"` // Exa (unwrapped live, 2026-11-15)
	}
	if json.Unmarshal(body, &v) != nil {
		return nil
	}
	rows := v.Companies
	if v.Output != nil {
		rows = v.Output.Companies
	}
	if len(rows) == 0 {
		rows = v.Results
	}
	if len(rows) == 0 && v.Output != nil {
		rows = v.Output.Results
	}
	var out []ai.ProspectCandidate
	byDomain := map[string]bool{}
	for _, r := range rows {
		r = withExaEntity(r)
		domain := companyDomain(firstString(r, "domain", "website", "website_url", "websiteUrl", "url", "homepage", "company_domain"))
		name := firstString(r, "name", "company_name", "companyName", "organization", "title")
		if domain == "" || name == "" || byDomain[domain] {
			continue
		}
		byDomain[domain] = true
		out = append(out, ai.ProspectCandidate{
			Name:        truncateRunes(name, maxProspectName), // provider text: it reaches prompts and the lead's title
			Domain:      domain,
			Description: truncateRunes(firstString(r, "description", "short_description", "shortDescription", "summary", "about", "tagline"), maxProspectDescription),
			Employees:   enrich.ParseSize(firstValue(r, "employees", "employee_count", "employeeCount", "headcount", "size", "company_size", "estimated_num_employees")),
			Country:     firstString(r, "country", "country_code", "countryCode", "hq_country"),
		})
	}
	return out
}

// withExaEntity flattens an Exa result's company entity (name, description,
// workforce.total, headquarters.country) onto the row's own keys.
func withExaEntity(r map[string]any) map[string]any {
	ents, _ := r["entities"].([]any)
	if len(ents) == 0 {
		return r
	}
	ent, _ := ents[0].(map[string]any)
	props, _ := ent["properties"].(map[string]any)
	if props == nil {
		return r
	}
	out := make(map[string]any, len(r)+4)
	for k, v := range r {
		out[k] = v
	}
	for _, k := range []string{"name", "description"} {
		if s, ok := props[k].(string); ok && s != "" {
			out[k] = s
		}
	}
	if wf, ok := props["workforce"].(map[string]any); ok && wf["total"] != nil {
		out["employees"] = wf["total"]
	}
	if hq, ok := props["headquarters"].(map[string]any); ok && hq["country"] != nil {
		out["country"] = hq["country"]
	}
	return out
}

// directoryHosts are profile sites some providers give as the website.
var directoryHosts = map[string]bool{"linkedin.com": true, "crunchbase.com": true, "facebook.com": true, "twitter.com": true, "x.com": true, "instagram.com": true, "google.com": true}

// companyDomain turns a website value into its registrable domain
// ("https://www.eu.acme.co.uk/about" -> "acme.co.uk"), "" when unusable.
func companyDomain(raw string) string {
	d := cleanDomain(raw)
	if d == "" || !strings.Contains(d, ".") {
		return ""
	}
	reg, err := publicsuffix.EffectiveTLDPlusOne(d)
	if err != nil || directoryHosts[reg] {
		return ""
	}
	return reg
}

func firstValue(r map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := r[k]; ok && v != nil && v != "" {
			return v
		}
	}
	return nil
}

func firstString(r map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := r[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// newProspects drops companies already judged, the workspace's own company
// and its named competitors.
func newProspects(in []ai.ProspectCandidate, seen map[string]bool, ownURL string, competitors []string) []ai.ProspectCandidate {
	own := companyDomain(ownURL)
	var out []ai.ProspectCandidate
next:
	for _, c := range in {
		if seen[c.Domain] || c.Domain == own {
			continue
		}
		for _, comp := range competitors {
			if sameCompany(comp, c.Name) || companyDomain(comp) == c.Domain {
				continue next
			}
		}
		out = append(out, c)
	}
	return out
}

// insertProspect stores a confirmed prospect as a scored mention with a lead.
// It returns the mention's id ("" when nothing was stored).
func (m *Monitor) insertProspect(ctx context.Context, wsID, pid string, seg ai.CustomerSegment, c ai.ProspectCandidate, fit ai.ProspectFit, person foundPerson, contact *enrich.Result, brief *ai.ResearchBrief) string {
	contactTitle := person.Title
	if contact.Title != "" {
		contactTitle = contact.Title
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s fits your customer segment %q: %s", c.Name, seg.Name, fit.Reason)
	fmt.Fprintf(&b, "\n\nWho to contact: %s, %s at %s.", person.Name, contactTitle, c.Name)
	email := emailLabel(contact.EmailStatus)
	b.WriteString(emailCaveat(email))
	b.WriteString(researchNote(brief))
	if seg.Pain != "" {
		b.WriteString("\n\nWhat this segment struggles with: " + seg.Pain)
	}
	companyURL := "https://" + c.Domain
	meta := map[string]any{"signal": "prospect", "company": c.Name, "company_url": companyURL, "segment": seg.Name,
		"fit_reason": fit.Reason, "contact_title": contactTitle, "confirmed": true, "email_status": email}
	if brief != nil {
		meta["research"] = brief
	}
	alert := m.insertMention(ctx, database.CreateMentionParams{
		WorkspaceID: wsID, Platform: string(database.PlatformTypeExa), PlatformID: pid, Url: companyURL,
		Title: pgtextPtr(fmt.Sprintf("%s fits %q", c.Name, seg.Name)), Content: b.String(),
		AuthorUsername: pgtextPtr(person.Name), AuthorProfileUrl: pgtextPtr(person.ProfileURL),
		Status:            database.MentionStatusNew,
		PlatformMetadata:  jsonBytes(meta),
		EngagementMetrics: jsonBytes(map[string]any{}),
		KeywordMatches:    []string{"prospect"},
		PlatformCreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}, "prospect")
	if alert == nil {
		return "" // duplicate or muted
	}
	// Scored on fit like a hiring lead (no posting, so no country or date),
	// never under the lead threshold: the contact was confirmed.
	score := max(leadScoreThreshold, hiringLeadScore(leadFit{
		contact: contactFit(contactTitle, seg.BuyerTitles, c.Employees, ""), sizeFits: c.Employees > 0, email: email,
	}, time.Now())-prospectScoreDiscount)
	reasoning := fmt.Sprintf("Fits the customer segment %q: %s", seg.Name, fit.Reason)
	res := &ai.ClassifyResult{
		Intent: string(database.IntentTypeGeneral), RelevanceScore: score, ConversionProbability: 0.1,
		Reasoning: reasoning, AwarenessLevel: "problem_aware",
		IntentTags: []string{"prospect"}, Sentiment: "neutral", PostType: "other", AuthorRole: "buyer",
	}
	if _, err := m.q.UpdateMentionScoring(ctx, database.UpdateMentionScoringParams{
		ID: alert.ID, WorkspaceID: wsID,
		Intent:                database.NullIntentType{IntentType: database.IntentTypeGeneral, Valid: true},
		ConversionProbability: pgtype.Float4{Float32: float32(res.ConversionProbability), Valid: true},
		RelevanceScore:        pgtype.Float4{Float32: float32(score), Valid: true},
		ScoringMetadata:       jsonBytes(map[string]any{"stage": "prospect", "auto_scored": true, "reasoning": reasoning, "author_role": "buyer", "intent_tags": res.IntentTags, "confirmed": true, "segment": seg.Name}),
		AwarenessLevel:        pgtype.Text{String: res.AwarenessLevel, Valid: true},
	}); err != nil {
		// The lead is still created: the mention now exists, so this company
		// would otherwise never become a lead.
		m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("prospects: store score")
	} else {
		m.storeIntentTags(ctx, wsID, alert.ID, res)
	}
	m.createCompanyLead(ctx, *alert, companyLead{company: c.Name, person: person, contact: contact,
		tag: "prospect", score: score, email: email, intent: database.IntentTypeGeneral})
	return alert.ID
}

func (m *Monitor) loadProspectState(ctx context.Context, wsID string) (prospectState, bool) {
	var st prospectState
	raw, err := m.q.GetWorkspaceSettingsKey(ctx, wsID, prospectSettingsKey)
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("prospects: load state")
		return st, false
	}
	if len(raw) > 0 && json.Unmarshal(raw, &st) != nil {
		m.logger.Warn().Str("workspace_id", wsID).Msg("prospects: bad state")
		return st, false
	}
	return st, true
}

func (m *Monitor) saveProspectState(ctx context.Context, wsID string, st prospectState) {
	if err := m.q.SetWorkspaceSettingsKey(ctx, wsID, prospectSettingsKey, jsonBytes(st)); err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("prospects: save state")
	}
}
