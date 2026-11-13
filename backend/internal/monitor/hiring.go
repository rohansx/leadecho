package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/net/publicsuffix"

	"leadecho/internal/ai"
	"leadecho/internal/billing"
	"leadecho/internal/crmsync"
	"leadecho/internal/database"
	"leadecho/internal/enrich"
	"leadecho/internal/treg"
)

// Hiring signals: for products where an open job means the company now has
// the problem (hiring engineers -> needs to screen engineers), find companies
// that posted a matching job this week and the person there who would buy.
// One lead per company per month, with a real name and LinkedIn profile.

const (
	tregJobsTool    = "anyapi.linkedin.search.jobs"
	tregPeopleTool  = "treg.people.search"
	tregCompanyTool = "treg.companies.enrich"

	hiringSettingsKey = "hiring_signals"
	hiringInterval    = 24 * time.Hour
	hiringRetry       = time.Hour // wait after a run in which every job search failed
	// ponytail: fixed daily budget (jobs: 1 call per role; people: 1 call per
	// new company). Make it a plan allowance if customers want more volume.
	maxHiringCompaniesPerRun = 8 // Pro
	starterHiringCompanies   = 3 // Starter: sized so its credits also cover its 15 keywords
	// Company size lookups per run: most job results are large employers, so
	// many candidates are checked to find a few that fit.
	maxHiringSizeChecksPerRun = 30 // Pro
	starterHiringSizeChecks   = 10 // Starter
	maxHiringSkipped          = 500
	maxHiringScreened         = 60 // postings handed to the screen in one call

	// Confirmed: a decision-maker whose own profile names this company, with a
	// usable email, at a company of known, fitting size. Only these are leads.
	hiringScoreConfirmed = 7.5
	// A confirmed contact whose email could not be verified (catch-all mail
	// server) is still a lead, ranked under the ones with a verified address.
	hiringScoreUnverifiedEmail = 6.5
	// From this size up, the person to write to is whoever is hiring for the
	// role, not a company-wide executive who never sees one requisition.
	hiringTeamContactSize = 500
	// Candidates tried per company when the first can't be confirmed.
	maxContactTries = 2
	// Unconfirmed signals stay below the lead threshold (6): visible under
	// "All conversations" with the reason, but not presented as a lead.
	hiringScoreUnconfirmed = 5.0
)

// HiringPlanner decides whether job postings signal a need for a product.
type HiringPlanner interface {
	PlanHiringSignals(ctx context.Context, workspaceID, product, overview, audience string) (*ai.HiringPlan, error)
	DraftOutreach(ctx context.Context, workspaceID string, o ai.Outreach) (string, error)
	// ScreenHiringJobs returns the postings worth a paid lookup: the right
	// role, an employer hiring for itself, in the customer's market.
	ScreenHiringJobs(ctx context.Context, workspaceID string, in ai.HiringScreen) ([]ai.ProspectFit, error)
	// ResearchCompany searches the web for what a company does and what is
	// happening there now. Every fact it returns carries its source.
	ResearchCompany(ctx context.Context, workspaceID string, in ai.CompanyResearch) (*ai.ResearchBrief, error)
}

// researchTimeout bounds one company's web research (several searches).
const researchTimeout = 3 * time.Minute

// ContactVerifier looks a contact up (title, current employer, company size,
// email) so a signal is only called a lead when the contact checks out.
type ContactVerifier interface {
	Enrich(ctx context.Context, ws string, s enrich.Subject) (*enrich.Result, error)
}

// SetContactVerifier enables confirmation; without it no signal is confirmed.
func (m *Monitor) SetContactVerifier(v ContactVerifier) { m.contactVerifier = v }

// SetHiringPlanner turns the hiring-signal source on.
func (m *Monitor) SetHiringPlanner(p HiringPlanner) { m.hiringPlanner = p }

// hiringState is stored under workspaces.settings.hiring_signals.
type hiringState struct {
	Plan    *ai.HiringPlan `json:"plan,omitempty"`
	LastRun time.Time      `json:"last_run"`
	// Skip remembers companies that are over the plan's size ceiling or were
	// rejected on insert, so they are not looked up (and paid for) again.
	Skip []string `json:"skip,omitempty"`
}

type jobPost struct {
	ID         string `json:"id"`
	Company    string `json:"company"`
	CompanyURL string `json:"companyUrl"`
	Title      string `json:"title"`
	Location   string `json:"location"`
	URL        string `json:"url"`
	CreatedUTC int64  `json:"createdUtc"`
}

type foundPerson struct {
	Name       string `json:"name"`
	Title      string `json:"title"`
	Company    string `json:"company"`
	ProfileURL string `json:"profile_url"`
}

// scanHiring runs at most once a day per workspace. It needs paid data
// (every call goes through the treg meter) and a brand profile.
func (m *Monitor) scanHiring(ctx context.Context, wsID string) {
	// Checked before claiming today's run: a run that can't make paid calls
	// must not count as "ran today".
	if m.hiringPlanner == nil || !m.treg.Enabled() || m.credits.PaidPlatformsLocked(ctx, wsID) || m.credits.PaidDataPaused(ctx) {
		return
	}
	bp, err := m.q.GetBrandProfile(ctx, wsID)
	if err != nil || bp.Product == "" {
		return // not onboarded yet: try again next tick
	}
	// Atomic claim: the api and the worker can both run the monitor, and only
	// one of them may spend today's budget. Claiming first also means a
	// failure below is retried tomorrow, not on every tick.
	claimed, err := m.q.ClaimSettingsRun(ctx, wsID, hiringSettingsKey, hiringClaimInterval(ctx))
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("hiring: claim failed")
		return
	}
	if !claimed {
		return
	}
	st, ok := m.loadHiringState(ctx, wsID)
	if !ok {
		return
	}
	if st.Plan == nil {
		plan, err := m.hiringPlanner.PlanHiringSignals(ctx, wsID, bp.Product, bp.Overview, bp.TargetAudience)
		if err != nil {
			m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("hiring: plan failed")
			return
		}
		st.Plan = plan
		if !m.saveHiringState(ctx, wsID, st) {
			return
		}
	}
	if !st.Plan.Applies {
		return
	}

	month := st.LastRun.Format("2006-01")
	skip := map[string]bool{}
	for _, k := range st.Skip {
		skip[k] = true
	}
	remember := func(key string) { // never look this company up (and pay) again
		skip[key] = true
		if len(st.Skip) < maxHiringSkipped {
			st.Skip = append(st.Skip, key)
		}
	}
	// The larger daily budget is Pro's; any other plan, or an unreadable one,
	// gets the smaller.
	dailyCap, checkCap := starterHiringCompanies, starterHiringSizeChecks
	if bs, err := m.q.GetBillingState(ctx, wsID); err == nil && bs.Plan == "pro" {
		dailyCap, checkCap = maxHiringCompaniesPerRun, maxHiringSizeChecksPerRun
	}
	created, sizeChecks, lookups := 0, 0, 0
	jobs, searched := m.hiringJobs(ctx, wsID, st.Plan.Roles)
	if !searched {
		// Every job search failed (out of credits, spend cap, provider down):
		// try again in an hour, not tomorrow. Not sooner: a provider that
		// bills and then times out would be paid on every tick.
		st.LastRun = time.Now().UTC().Add(hiringRetry - hiringInterval)
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		m.saveHiringState(saveCtx, wsID, st)
		return
	}
	var fresh []jobPost
	for _, job := range jobs {
		// Too old would be dropped on insert, after its lookups were paid for.
		if skip[companyKey(job.Company)] || !validHTTPURL(job.URL) || (job.CreatedUTC > 0 && time.Since(time.Unix(job.CreatedUTC, 0)) > maxMentionAge) {
			continue
		}
		if exists, err := m.q.MentionExists(ctx, wsID, string(database.PlatformTypeLinkedin), hiringPlatformID(job.Company, month)); err != nil || exists {
			continue
		}
		if fresh = append(fresh, job); len(fresh) == maxHiringScreened {
			break
		}
	}
	// One model call over the whole list, before anything is paid for: job
	// search returns agencies, internships, other functions and postings in
	// other languages, and every one of them used to cost a company lookup.
	fits, err := m.hiringPlanner.ScreenHiringJobs(ctx, wsID, ai.HiringScreen{
		Product: bp.Product, Overview: bp.Overview, Audience: bp.TargetAudience,
		Roles: st.Plan.Roles, MaxCompanySize: st.Plan.MaxCompanySize, Jobs: screenInput(fresh),
	})
	if err != nil {
		// Unscreened postings are what filled the inbox with non-buyers:
		// better no leads today than those. Tomorrow's run tries again.
		m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("hiring: screen failed")
		return
	}
	jobs, dropped := screenedJobs(fresh, fits)
	// Dropped postings are not remembered: a company that posted an internship
	// today may post a real opening next week, and screening again costs no
	// paid data.
	m.logger.Info().Int("kept", len(jobs)).Int("dropped", len(dropped)).Str("workspace_id", wsID).Msg("hiring: postings screened")
	for _, job := range jobs {
		// The cap counts paid people searches, not stored leads: a search
		// whose lead is then rejected still cost money.
		if lookups >= dailyCap || ctx.Err() != nil {
			break
		}
		key := companyKey(job.Company)
		pid := hiringPlatformID(job.Company, month)
		// Size and website of the hiring company: the size gates the plan's
		// market, the website is what the contact's employer is checked against.
		if sizeChecks >= checkCap {
			break
		}
		sizeChecks++
		co, err := m.companyInfo(ctx, wsID, job)
		if err != nil {
			break // out of credits, paused or provider down: without sizes every company would pass
		}
		if st.Plan.MaxCompanySize > 0 && co.size > st.Plan.MaxCompanySize {
			remember(key)
			continue
		}
		if st.Plan.MaxCompanySize > 0 && co.size == 0 && co.domain == "" {
			// Nothing is known about the company, so its size can't come from
			// a contact either (that needs the websites to match): record it
			// without spending one of the day's people searches. With a known
			// website the contact's profile can still supply the size.
			if id := m.insertHiringSignal(ctx, wsID, pid, job, foundPerson{}, st.Plan,
				contactCheck{reason: "company size unknown, so it may be outside your market"}, nil, nil); id == "" {
				remember(key)
			} else {
				created++
			}
			continue
		}
		lookups++
		candidates, err := m.hiringBuyers(ctx, wsID, st.Plan.BuyerTitles, job, co.size)
		if err != nil {
			break // same: don't store "nobody found" leads because a call failed
		}
		person, contact, check, capped := chooseContact(candidates,
			func(c foundPerson) (*enrich.Result, bool) { return m.verifyContact(ctx, wsID, job, c) },
			func(c foundPerson, res *enrich.Result) contactCheck {
				return confirmContact(job.Company, co, st.Plan.MaxCompanySize, c, res)
			})
		if capped && person.Name == "" {
			break // today's contact lookups ran out before anyone was checked: retry tomorrow
		}
		if check.tooBig {
			remember(key)
			continue
		}
		// Only a confirmed lead is worth researching: the brief makes the
		// lead view and the first message specific to this company.
		var brief *ai.ResearchBrief
		if check.confirmed {
			brief = m.researchLead(ctx, wsID, hiringResearch(bp, st.Plan, job, person))
			if ctx.Err() != nil {
				// Shutting down mid-research: the insert would fail and the
				// company would be skipped for good. Leave it for tomorrow.
				m.refundResearch(ctx, wsID, brief)
				break
			}
		}
		if ruled := afterResearch(check, brief); !ruled.confirmed && check.confirmed {
			// The company's own pages show it would not buy: kept as a
			// record, not charged, not written to.
			m.refundResearch(ctx, wsID, brief)
			check, brief = ruled, nil
		}
		mentionID := m.insertHiringSignal(ctx, wsID, pid, job, person, st.Plan, check, contact, brief)
		if mentionID == "" {
			m.refundResearch(ctx, wsID, brief) // nothing was stored, so nothing is charged
			remember(key)                      // muted or rejected on insert: it would be rejected again tomorrow
			continue
		}
		created++
		if check.confirmed {
			m.draftOutreach(ctx, wsID, mentionID, ai.Outreach{
				Product: bp.Product, Overview: bp.Overview, Positioning: bp.Positioning,
				ContactName: person.Name, ContactTitle: person.Title, Company: job.Company, JobTitle: job.Title,
				Research: brief,
			})
		}
		if capped {
			break // today's contact lookups are used up: later companies could not be confirmed either
		}
	}
	// Outlive a shutdown: losing the skip list means paying for the same lookups again.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	m.saveHiringState(saveCtx, wsID, st)
	m.logger.Info().Int("count", created).Int("size_checks", sizeChecks).Int("people_searches", lookups).Str("workspace_id", wsID).Msg("hiring: scan done")
}

// hiringCompany is what the provider knows about the company behind a job.
type hiringCompany struct {
	size   int    // employees, 0 = unknown (kept: small companies are the ones providers miss)
	domain string // website domain, "" = unknown
}

// companyInfo looks the hiring company up by name. Not by its LinkedIn page:
// the provider that answers page lookups returned the wrong company in live
// testing (JPMorganChase came back as a one-person events firm). A result
// for a differently named company is discarded as unknown.
func (m *Monitor) companyInfo(ctx context.Context, wsID string, job jobPost) (hiringCompany, error) {
	resp, err := m.treg.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tregCompanyTool, Body: map[string]string{"name": job.Company}})
	if err != nil {
		return hiringCompany{}, err
	}
	return parseCompanyInfo(resp.Body, job.Company), nil
}

// parseCompanyInfo reads {output:{name,domain,employees}} (or unwrapped).
// want is the company asked for: an answer about another company is dropped.
func parseCompanyInfo(body []byte, want string) hiringCompany {
	type row struct {
		Name      string `json:"name"`
		Employees any    `json:"employees"`
		Domain    string `json:"domain"`
	}
	var v struct {
		Output *row `json:"output"`
		row
	}
	if json.Unmarshal(body, &v) != nil {
		return hiringCompany{}
	}
	got := v.row
	if v.Output != nil {
		got = *v.Output
	}
	if !sameCompany(want, got.Name) {
		return hiringCompany{}
	}
	return hiringCompany{size: enrich.ParseSize(got.Employees), domain: cleanDomain(got.Domain)}
}

// cleanDomain normalises "https://www.Acme.io/about" and "WWW.acme.io" to "acme.io".
func cleanDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	if i := strings.Index(d, "://"); i >= 0 {
		d = d[i+3:]
	}
	d = strings.TrimPrefix(d, "www.")
	if i := strings.IndexAny(d, "/?#:"); i >= 0 {
		d = d[:i]
	}
	return d
}

// validHTTPURL accepts only absolute http(s) URLs: these come from a third
// party and end up as links in the app and in email.
func validHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

// linkedInProfileURL reports whether raw is an https LinkedIn member profile.
func linkedInProfileURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Host)
	return (host == "linkedin.com" || host == "www.linkedin.com") && strings.HasPrefix(u.Path, "/in/")
}

// loadHiringState returns ok=false when the state can't be read: without it
// the skip list is unknown, so nothing is spent.
func (m *Monitor) loadHiringState(ctx context.Context, wsID string) (hiringState, bool) {
	var st hiringState
	raw, err := m.q.GetWorkspaceSettingsKey(ctx, wsID, hiringSettingsKey)
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("hiring: load state")
		return st, false
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &st); err != nil {
			m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("hiring: bad state")
			return st, false
		}
	}
	return st, true
}

func (m *Monitor) saveHiringState(ctx context.Context, wsID string, st hiringState) bool {
	if err := m.q.SetWorkspaceSettingsKey(ctx, wsID, hiringSettingsKey, jsonBytes(st)); err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("hiring: save state")
		return false
	}
	return true
}

// hiringJobs searches each role and returns one job per company, newest
// first. searched is false when no search succeeded at all.
func (m *Monitor) hiringJobs(ctx context.Context, wsID string, roles []string) (jobs []jobPost, searched bool) {
	var all []jobPost
	for _, role := range roles {
		resp, err := m.treg.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tregJobsTool,
			Body: map[string]string{"query": role, "postedLimit": "week"}})
		if err != nil {
			if !treg.IsOperatorError(err) {
				m.logger.Debug().Err(err).Str("role", role).Msg("hiring: job search failed")
			}
			continue
		}
		searched = true
		all = append(all, parseJobs(resp.Body)...)
	}
	return onePerCompany(all), searched
}

// parseJobs reads {output:{data:{items:[...]}}} (and the unwrapped forms).
func parseJobs(body []byte) []jobPost {
	var v struct {
		Output *struct {
			Data struct {
				Items []jobPost `json:"items"`
			} `json:"data"`
		} `json:"output"`
		Data struct {
			Items []jobPost `json:"items"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &v) != nil {
		return nil
	}
	if v.Output != nil {
		return v.Output.Data.Items
	}
	return v.Data.Items
}

// onePerCompany keeps each company's newest job, newest companies first.
func onePerCompany(jobs []jobPost) []jobPost {
	best := map[string]jobPost{}
	var order []string
	for _, j := range jobs {
		key := companyKey(j.Company)
		if key == "" || j.URL == "" {
			continue
		}
		cur, seen := best[key]
		if !seen {
			order = append(order, key)
		}
		if !seen || j.CreatedUTC > cur.CreatedUTC {
			best[key] = j
		}
	}
	out := make([]jobPost, 0, len(order))
	for _, k := range order {
		out = append(out, best[k])
	}
	for i := 1; i < len(out); i++ { // insertion sort: tiny list, keeps ties in search order
		for j := i; j > 0 && out[j].CreatedUTC > out[j-1].CreatedUTC; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

var nonAlnumRe = regexp.MustCompile(`[^a-z0-9]+`)

func companyKey(name string) string {
	return strings.Trim(nonAlnumRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

func hiringPlatformID(company, month string) string {
	return "hiring-" + companyKey(company) + "-" + month
}

// hiringBuyers finds up to maxContactTries people at the company who could
// buy, best first. No candidates with a nil error means the search worked and
// found nobody suitable; an error means the search itself failed.
func (m *Monitor) hiringBuyers(ctx context.Context, wsID, titles string, job jobPost, companySize int) ([]foundPerson, error) {
	resp, err := m.treg.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tregPeopleTool,
		Body: map[string]string{"q": buyerQuery(titles, job, companySize)}})
	if err != nil {
		if !treg.IsOperatorError(err) {
			m.logger.Debug().Err(err).Str("company", job.Company).Msg("hiring: people search failed")
		}
		return nil, err
	}
	return pickBuyers(parsePeople(resp.Body), job.Company, maxContactTries), nil
}

// chooseContact checks candidates in order until one is confirmed. The first
// is not always right (moved on, a namesake, no email), so the next is tried
// before the company is given up; if none confirms, the first one's result is
// kept. capped reports that the day's contact lookups ran out: whatever was
// already checked (and paid for) is still returned so it can be stored.
// With no candidates the check carries the "nobody found" reason.
func chooseContact(candidates []foundPerson, verify func(foundPerson) (*enrich.Result, bool), confirm func(foundPerson, *enrich.Result) contactCheck) (person foundPerson, contact *enrich.Result, check contactCheck, capped bool) {
	check = confirm(foundPerson{}, nil)
	for i, cand := range candidates {
		res, hitCap := verify(cand)
		if hitCap {
			return person, contact, check, true
		}
		ck := confirm(cand, res)
		if i == 0 || ck.confirmed || ck.tooBig {
			person, contact, check = cand, res, ck
		}
		if ck.confirmed || ck.tooBig {
			break
		}
	}
	return person, contact, check, false
}

// buyerQuery words the people search. At a small company the buyer titles
// (CTO, VP Engineering) are the people hiring. At a larger one those
// executives do not handle a single opening, so the search names the role:
// live, that returned the company's technical recruiters and engineering
// managers where the plain title search returned an unrelated VP.
func buyerQuery(titles string, job jobPost, companySize int) string {
	if companySize >= hiringTeamContactSize && job.Title != "" {
		title := job.Title // third-party text: keep the query short
		if rs := []rune(title); len(rs) > 120 {
			title = string(rs[:120])
		}
		return "manager or recruiter hiring for " + title + " at " + job.Company
	}
	return titles + " at " + job.Company
}

func parsePeople(body []byte) []foundPerson {
	var v struct {
		Output *struct {
			People []foundPerson `json:"people"`
		} `json:"output"`
		People []foundPerson `json:"people"`
	}
	if json.Unmarshal(body, &v) != nil {
		return nil
	}
	if v.Output != nil {
		return v.Output.People
	}
	return v.People
}

// decisionTitleRe matches titles of people who choose or recommend tools.
// People search answers with anyone at the company when it finds no one with
// the asked-for title, and an individual contributor is not a buyer.
// ponytail: word list, English only. Ask the model to judge the title if
// other languages or unusual titles matter.
var decisionTitleRe = regexp.MustCompile(`(?i)\b(chief|c[etofm]o|founder|owner|president|vp|vice president|head|director|manager|lead|recruit\w*|talent|people|partner)\b`)

// pickBuyer takes the first result whose current company is this one: people
// search also returns former employees and similarly named companies.
func pickBuyer(people []foundPerson, company string) (foundPerson, bool) {
	if got := pickBuyers(people, company, 1); len(got) == 1 {
		return got[0], true
	}
	return foundPerson{}, false
}

// pickBuyers returns the first n such results, in search order.
func pickBuyers(people []foundPerson, company string, n int) []foundPerson {
	want := companyKey(company)
	var out []foundPerson
	for _, p := range people {
		if len(out) == n {
			break
		}
		got := companyKey(p.Company)
		if p.Name == "" || !linkedInProfileURL(p.ProfileURL) || got == "" || !decisionTitleRe.MatchString(p.Title) {
			continue
		}
		if got == want || strings.HasPrefix(got, want+"-") || strings.HasPrefix(want, got+"-") {
			out = append(out, p)
		}
	}
	return out
}

// verifyContact runs the paid lookup on the contact people-search proposed.
// A nil result means no contact or no verifier: the signal stays unconfirmed.
// capped reports that the workspace's daily enrichment cap is reached.
//
// Lookups that end unconfirmed are not stored, so they do not count toward
// ENRICH_DAILY_CAP; this path is bounded by the per-run company cap instead.
func (m *Monitor) verifyContact(ctx context.Context, wsID string, job jobPost, person foundPerson) (res *enrich.Result, capped bool) {
	if person.Name == "" || m.contactVerifier == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	res, err := m.contactVerifier.Enrich(ctx, wsID, enrich.Subject{
		// No Company here: the lookup must report the employer on the
		// contact's own profile, not echo the one we hope for.
		Name: person.Name, Platform: string(database.PlatformTypeLinkedin), ProfileURL: person.ProfileURL,
	})
	if errors.Is(err, enrich.ErrDailyCap) {
		return nil, true
	}
	if err != nil && res == nil {
		m.logger.Debug().Err(err).Str("company", job.Company).Msg("hiring: contact lookup failed")
	}
	return res, false
}

type contactCheck struct {
	confirmed bool
	tooBig    bool   // over the plan's company-size ceiling: drop the signal
	reason    string // why it is not confirmed, in plain words
}

// confirmContact decides whether a signal is a lead. People search and job
// boards are both wrong often enough (a namesake company, someone who has
// moved on, an employer too large) that every claim is checked against the
// contact's own profile before anyone is told to write to them.
func confirmContact(company string, co hiringCompany, maxSize int, person foundPerson, c *enrich.Result) contactCheck {
	if person.Name == "" {
		return contactCheck{reason: "no decision-maker found at this company yet"}
	}
	if c == nil || c.FullName == "" {
		return contactCheck{reason: "could not look up " + person.Name + "'s profile"}
	}
	// The contact's company size only speaks for the hiring company once we
	// know they are the same company: a namesake's size must not get the real
	// one dropped as too big.
	employerOK := sameEmployer(co.domain, c)
	size := co.size
	if size == 0 && employerOK {
		size = c.CompanySize
	}
	switch {
	case maxSize > 0 && size > maxSize:
		return contactCheck{tooBig: true}
	case !employerOK:
		switch {
		case c.Company == "":
			return contactCheck{reason: fmt.Sprintf("%s's profile lists no current employer", person.Name)}
		case sameCompany(company, c.Company):
			// The names agree, but the websites don't confirm it (one is
			// unknown, or they differ): it may be a namesake.
			return contactCheck{reason: fmt.Sprintf("could not verify that %s's employer is this %s and not another company of the same name", person.Name, company)}
		}
		return contactCheck{reason: fmt.Sprintf("%s's profile lists %s, not %s", person.Name, c.Company, company)}
	case maxSize > 0 && size == 0:
		return contactCheck{reason: "company size unknown, so it may be outside your market"}
	case c.Email == "" || c.EmailStatus == "invalid":
		return contactCheck{reason: "no working email found for " + person.Name}
	}
	return contactCheck{confirmed: true}
}

// sameEmployer checks the contact's employer against the hiring company by
// website domain. Both must be known: two companies can share a name
// ("Prophetic" the software firm and "Prophetic" the ultrasound firm) but not
// a domain, so a name match alone never confirms a contact.
func sameEmployer(companyDomain string, c *enrich.Result) bool {
	d := cleanDomain(c.CompanyDomain)
	if d == "" || companyDomain == "" {
		return false
	}
	return d == companyDomain || subdomainOf(d, companyDomain) || subdomainOf(companyDomain, d)
}

// subdomainOf reports whether sub is a subdomain of parent (eng.acme.com of
// acme.com). parent must itself be a registrable domain: "acme.co.uk" is not
// a subdomain of "co.uk", nor "x.github.io" of "github.io".
func subdomainOf(sub, parent string) bool {
	if !strings.HasSuffix(sub, "."+parent) {
		return false
	}
	reg, err := publicsuffix.EffectiveTLDPlusOne(parent)
	return err == nil && reg == parent
}

var (
	parenRe     = regexp.MustCompile(`\([^)]*\)`)
	legalSuffix = map[string]bool{"inc": true, "llc": true, "ltd": true, "limited": true, "corp": true, "corporation": true, "co": true, "company": true, "gmbh": true, "plc": true, "sa": true, "bv": true, "pvt": true}
)

// sameCompany reports whether two names are the same employer. Bracketed
// notes and legal suffixes are ignored ("LVT (LiveView Technologies)" = "LVT",
// "Acme Inc." = "Acme"); anything else must match exactly, so "Acme" is not
// "Acme Health" and "JPMorganChase" is not "JPMorganChase Corporate Challenge".
func sameCompany(a, b string) bool {
	ka, kb := strictCompanyKey(a), strictCompanyKey(b)
	return ka != "" && ka == kb
}

func strictCompanyKey(name string) string {
	words := strings.Split(companyKey(parenRe.ReplaceAllString(name, " ")), "-")
	for len(words) > 1 && legalSuffix[words[len(words)-1]] {
		words = words[:len(words)-1]
	}
	return strings.Join(words, "-")
}

// insertHiringSignal stores the signal. It skips the LLM scorer on purpose:
// the scorer treats job posts as noise, and here the job post is the
// evidence. Only a confirmed signal is scored as a lead and gets a lead row.
// It returns the new mention's id ("" when nothing was stored).
func (m *Monitor) insertHiringSignal(ctx context.Context, wsID, pid string, job jobPost, person foundPerson, plan *ai.HiringPlan, check contactCheck, contact *enrich.Result, brief *ai.ResearchBrief) string {
	title := fmt.Sprintf("%s is hiring: %s", job.Company, job.Title)
	var b strings.Builder
	fmt.Fprintf(&b, "%s posted a job for %q", job.Company, job.Title)
	if job.Location != "" {
		fmt.Fprintf(&b, " (%s)", job.Location)
	}
	b.WriteString(" this week.")
	score, author, authorURL := hiringScoreUnconfirmed, job.Company, job.CompanyURL
	if person.Name != "" {
		author, authorURL = person.Name, person.ProfileURL
	}
	contactTitle := person.Title
	if check.confirmed {
		score = hiringLeadScore(contact)
		if contact.Title != "" {
			contactTitle = contact.Title
		}
		fmt.Fprintf(&b, "\n\nWho to contact: %s, %s at %s.", person.Name, contactTitle, job.Company)
		if contact.EmailStatus == "catchall" {
			b.WriteString(" Their email could not be verified (the company's mail server accepts any address), so it may bounce.")
		}
	} else {
		fmt.Fprintf(&b, "\n\nNot confirmed as a lead: %s.", check.reason)
	}
	b.WriteString(researchNote(brief))
	if plan.Why != "" {
		b.WriteString("\n\nWhy this is a signal: " + plan.Why)
	}

	var created pgtype.Timestamptz
	if job.CreatedUTC > 0 {
		created = pgtype.Timestamptz{Time: time.Unix(job.CreatedUTC, 0), Valid: true}
	}
	alert := m.insertMention(ctx, database.CreateMentionParams{
		WorkspaceID: wsID, Platform: string(database.PlatformTypeLinkedin), PlatformID: pid, Url: job.URL,
		Title: pgtextPtr(title), Content: b.String(),
		AuthorUsername: pgtextPtr(author), AuthorProfileUrl: pgtextPtr(authorURL),
		Status:            hiringSignalStatus(check),
		PlatformMetadata:  hiringMetadata(job, contactTitle, check, brief),
		EngagementMetrics: jsonBytes(map[string]any{}),
		KeywordMatches:    []string{"hiring signal"},
		PlatformCreatedAt: created,
	}, "hiring signal")
	if alert == nil {
		return "" // duplicate, muted or too old
	}

	reasoning := "Company posted a matching job this week. " + plan.Why
	if !check.confirmed {
		reasoning = "Company posted a matching job this week, but the contact is not confirmed: " + check.reason + "."
	}
	res := &ai.ClassifyResult{
		Intent: string(database.IntentTypeBuySignal), RelevanceScore: score, ConversionProbability: 0.3,
		Reasoning: reasoning, AwarenessLevel: "problem_aware",
		IntentTags: []string{"hiring"}, Sentiment: "neutral", PostType: "other", AuthorRole: "buyer",
	}
	if _, err := m.q.UpdateMentionScoring(ctx, database.UpdateMentionScoringParams{
		ID: alert.ID, WorkspaceID: wsID,
		Intent:                database.NullIntentType{IntentType: database.IntentTypeBuySignal, Valid: true},
		ConversionProbability: pgtype.Float4{Float32: float32(res.ConversionProbability), Valid: true},
		RelevanceScore:        pgtype.Float4{Float32: float32(score), Valid: true},
		ScoringMetadata:       jsonBytes(map[string]any{"stage": "hiring_signal", "auto_scored": true, "reasoning": res.Reasoning, "author_role": "buyer", "intent_tags": res.IntentTags, "confirmed": check.confirmed}),
		AwarenessLevel:        pgtype.Text{String: res.AwarenessLevel, Valid: true},
	}); err != nil {
		m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("hiring: store score")
		return alert.ID
	}
	m.storeIntentTags(ctx, wsID, alert.ID, res)
	if check.confirmed {
		m.createCompanyLead(ctx, *alert, companyLead{company: job.Company, person: person, contact: contact,
			tag: "hiring-signal", score: score, intent: database.IntentTypeBuySignal})
	}
	return alert.ID
}

// hiringMetadata is what the lead view reads about a hiring signal. The
// research (with its source links) is only present when something was found.
func hiringMetadata(job jobPost, contactTitle string, check contactCheck, brief *ai.ResearchBrief) []byte {
	meta := map[string]any{"signal": "hiring", "company": job.Company, "company_url": job.CompanyURL, "job_title": job.Title,
		"contact_title": contactTitle, "confirmed": check.confirmed, "unconfirmed_reason": check.reason}
	if brief != nil {
		meta["research"] = brief
	}
	return jsonBytes(meta)
}

// researchNote is the research as a paragraph of the lead's text (which the
// assistant, the digest and search all read). Sources stay in the metadata.
func researchNote(brief *ai.ResearchBrief) string {
	if brief == nil {
		return ""
	}
	var parts []string
	for _, p := range []string{brief.WhatTheyDo, brief.WhyNow} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "\n\nWhat a web search found about the company (not verified): " + strings.Join(parts, " ")
}

// refundResearch returns the research credits when its brief is not used.
func (m *Monitor) refundResearch(ctx context.Context, wsID string, brief *ai.ResearchBrief) {
	if brief != nil {
		m.credits.Refund(context.WithoutCancel(ctx), wsID, billing.CostResearch, billing.ReasonResearch)
	}
}

// researchLead looks the company up on the web for a confirmed lead. It is
// best effort: a lead without research is still a lead. Credits are returned
// when nothing usable comes back.
func (m *Monitor) researchLead(ctx context.Context, wsID string, in ai.CompanyResearch) *ai.ResearchBrief {
	if err := m.credits.Spend(ctx, wsID, billing.CostResearch, billing.ReasonResearch); err != nil {
		return nil
	}
	rctx, cancel := context.WithTimeout(ctx, researchTimeout)
	defer cancel()
	brief, err := m.hiringPlanner.ResearchCompany(rctx, wsID, in)
	if err != nil || brief == nil {
		m.credits.Refund(context.WithoutCancel(ctx), wsID, billing.CostResearch, billing.ReasonResearch)
		m.logger.Info().Err(err).Str("company", in.Company).Str("workspace_id", wsID).Msg("lead research: nothing found")
		return nil
	}
	m.logger.Info().Int("searches", brief.Searches).Int("facts", len(brief.Facts)).Str("company", in.Company).Str("workspace_id", wsID).Msg("lead research: company researched")
	return brief
}

// hiringResearch is the research request for a hiring signal's company.
func hiringResearch(bp database.BrandProfile, plan *ai.HiringPlan, job jobPost, person foundPerson) ai.CompanyResearch {
	return ai.CompanyResearch{
		Product: bp.Product, Overview: bp.Overview, Audience: bp.TargetAudience, MaxCompanySize: plan.MaxCompanySize,
		Company: job.Company, CompanyURL: job.CompanyURL, JobTitle: job.Title, ContactTitle: person.Title,
	}
}

// screenInput is the postings as the screen reads them.
func screenInput(jobs []jobPost) []ai.HiringJob {
	out := make([]ai.HiringJob, len(jobs))
	for i, j := range jobs {
		out[i] = ai.HiringJob{Company: j.Company, Title: j.Title, Location: j.Location}
	}
	return out
}

// screenedJobs splits the postings into those the screen kept (in their
// original, newest-first order) and those it dropped.
func screenedJobs(jobs []jobPost, fits []ai.ProspectFit) (kept, dropped []jobPost) {
	keep := map[int]bool{}
	for _, f := range fits {
		keep[f.Index] = true
	}
	for i, j := range jobs {
		if keep[i] {
			kept = append(kept, j)
		} else {
			dropped = append(dropped, j)
		}
	}
	return kept, dropped
}

// afterResearch withdraws a confirmed lead when the research found that the
// company would not buy.
func afterResearch(check contactCheck, brief *ai.ResearchBrief) contactCheck {
	if !check.confirmed || brief == nil || brief.NotABuyer == "" {
		return check
	}
	return contactCheck{reason: "a web search suggests this company is not a buyer: " + strings.TrimRight(brief.NotABuyer, ". ")}
}

// hiringSignalStatus keeps unconfirmed signals out of the inbox: they have no
// contact, email or draft, so there is nothing to act on. They are still
// stored (archived), which is what stops the company being looked up again.
func hiringSignalStatus(check contactCheck) database.MentionStatus {
	if check.confirmed {
		return database.MentionStatusNew
	}
	return database.MentionStatusArchived
}

// hiringLeadScore is a confirmed signal's score: lower when the email could
// not be verified.
func hiringLeadScore(contact *enrich.Result) float64 {
	if contact != nil && contact.EmailStatus == "catchall" {
		return hiringScoreUnverifiedEmail
	}
	return hiringScoreConfirmed
}

// companyLead is a confirmed contact at a company found by a signal
// (hiring or prospecting) rather than by a post they wrote.
type companyLead struct {
	company string
	person  foundPerson
	contact *enrich.Result
	tag     string // "hiring-signal" | "prospect"
	score   float64
	intent  database.IntentType
}

// createCompanyLead stores the confirmed contact as a lead, carrying the
// lookup that was already paid for so enrichment does not buy it again.
func (m *Monitor) createCompanyLead(ctx context.Context, alert mentionAlert, l companyLead) {
	lead, err := m.q.CreateLead(ctx, database.CreateLeadParams{
		WorkspaceID:  alert.WorkspaceID,
		MentionID:    pgUUID(alert.ID),
		Stage:        database.LeadStageProspect,
		ContactName:  pgtextPtr(l.contact.FullName),
		ContactEmail: pgtextPtr(l.contact.Email),
		Company:      pgtextPtr(l.company),
		Username:     pgtextPtr(l.person.Name),
		Platform:     database.NullPlatformType{PlatformType: database.PlatformTypeLinkedin, Valid: true},
		ProfileUrl:   pgtextPtr(l.person.ProfileURL),
		Tags:         []string{"auto-qualified", l.tag},
		Metadata:     jsonBytes(map[string]any{"auto_scored": true, "relevance": l.score, "intent": string(l.intent), "enrichment": l.contact}),
	})
	if err != nil {
		if !isDuplicateError(err) {
			m.logger.Error().Err(err).Str("mention_id", alert.ID).Str("tag", l.tag).Msg("create company lead")
		}
		return
	}
	if m.enricher != nil {
		m.enricher.EnrichLeadAsync(lead.ID, alert.WorkspaceID) // Person360; the paid lookup is skipped (already stored)
	}
	m.crm.Emit(alert.WorkspaceID, crmsync.Event{Type: crmsync.EventLeadCreated, LeadID: lead.ID})
}

// draftOutreach saves a first message to the contact as the lead's draft, so
// the inbox and the daily digest show something ready to send. Costs the
// same as a reply draft; refunded if writing fails.
func (m *Monitor) draftOutreach(ctx context.Context, wsID, mentionID string, o ai.Outreach) {
	if o.ContactName == "" {
		return // nobody to write to
	}
	if err := m.credits.Spend(ctx, wsID, billing.CostDraft, billing.ReasonDraft); err != nil {
		return // out of credits: the lead still stands without a draft
	}
	text, err := m.hiringPlanner.DraftOutreach(ctx, wsID, o)
	if err == nil {
		_, err = m.q.CreateReply(ctx, database.CreateReplyParams{
			MentionID: mentionID, WorkspaceID: wsID, Content: text, Status: database.ReplyStatusDraft,
			TemplateStyle: pgtype.Text{String: "outreach", Valid: true},
		})
	}
	if err != nil {
		m.credits.Refund(context.WithoutCancel(ctx), wsID, billing.CostDraft, billing.ReasonDraft) // a cancelled draft must still be refunded
		m.logger.Warn().Err(err).Str("mention_id", mentionID).Msg("outreach draft failed")
	}
}
