package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/billing"
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
	maxHiringCompaniesPerRun = 8
	// Company size lookups per run: most job results are large employers, so
	// many candidates are checked to find a few that fit.
	maxHiringSizeChecksPerRun = 30
	maxHiringSkipped          = 500

	hiringScoreWithPerson = 7.5 // above the lead threshold: a named buyer at a company with the need
	hiringScoreNoPerson   = 6.0
)

// HiringPlanner decides whether job postings signal a need for a product.
type HiringPlanner interface {
	PlanHiringSignals(ctx context.Context, workspaceID, product, overview, audience string) (*ai.HiringPlan, error)
	DraftOutreach(ctx context.Context, workspaceID string, o ai.Outreach) (string, error)
}

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
	claimed, err := m.q.ClaimSettingsRun(ctx, wsID, hiringSettingsKey, hiringInterval)
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
	for _, job := range jobs {
		// The cap counts paid people searches, not stored leads: a search
		// whose lead is then rejected still cost money.
		if lookups >= maxHiringCompaniesPerRun || ctx.Err() != nil {
			break
		}
		key := companyKey(job.Company)
		// Too old would be dropped on insert, after its lookups were paid for.
		if skip[key] || !validHTTPURL(job.URL) || (job.CreatedUTC > 0 && time.Since(time.Unix(job.CreatedUTC, 0)) > maxMentionAge) {
			continue
		}
		pid := hiringPlatformID(job.Company, month)
		if exists, err := m.q.MentionExists(ctx, wsID, string(database.PlatformTypeLinkedin), pid); err != nil || exists {
			continue
		}
		if st.Plan.MaxCompanySize > 0 {
			if sizeChecks >= maxHiringSizeChecksPerRun {
				break
			}
			sizeChecks++
			size, err := m.companySize(ctx, wsID, job.Company)
			if err != nil {
				break // out of credits, paused or provider down: without sizes every company would pass
			}
			if size > st.Plan.MaxCompanySize {
				remember(key)
				continue
			}
		}
		lookups++
		person, err := m.hiringBuyer(ctx, wsID, st.Plan.BuyerTitles, job.Company)
		if err != nil {
			break // same: don't store "nobody found" leads because a call failed
		}
		mentionID := m.insertHiringSignal(ctx, wsID, pid, job, person, st.Plan)
		if mentionID == "" {
			remember(key) // muted or rejected on insert: it would be rejected again tomorrow
			continue
		}
		created++
		m.draftOutreach(ctx, wsID, mentionID, bp, job, person)
	}
	// Outlive a shutdown: losing the skip list means paying for the same lookups again.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	m.saveHiringState(saveCtx, wsID, st)
	m.logger.Info().Int("count", created).Int("size_checks", sizeChecks).Int("people_searches", lookups).Str("workspace_id", wsID).Msg("hiring: scan done")
}

// companySize looks up a company's employee count. 0 with a nil error means
// the provider doesn't know it: such a company is kept, since small
// companies are the ones data providers miss.
func (m *Monitor) companySize(ctx context.Context, wsID, company string) (int, error) {
	resp, err := m.treg.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tregCompanyTool, Body: map[string]string{"name": company}})
	if err != nil {
		return 0, err
	}
	return parseCompanySize(resp.Body), nil
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

func parseCompanySize(body []byte) int {
	var v struct {
		Output *struct {
			Employees any `json:"employees"`
		} `json:"output"`
		Employees any `json:"employees"`
	}
	if json.Unmarshal(body, &v) != nil {
		return 0
	}
	if v.Output != nil {
		return enrich.ParseSize(v.Output.Employees)
	}
	return enrich.ParseSize(v.Employees)
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

// hiringBuyer finds the person with a buyer title who works at the company now.
// A person with no Name means the search worked and found nobody suitable;
// an error means the search itself failed.
func (m *Monitor) hiringBuyer(ctx context.Context, wsID, titles, company string) (foundPerson, error) {
	resp, err := m.treg.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tregPeopleTool,
		Body: map[string]string{"q": titles + " at " + company}})
	if err != nil {
		if !treg.IsOperatorError(err) {
			m.logger.Debug().Err(err).Str("company", company).Msg("hiring: people search failed")
		}
		return foundPerson{}, err
	}
	p, _ := pickBuyer(parsePeople(resp.Body), company)
	return p, nil
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
	want := companyKey(company)
	for _, p := range people {
		got := companyKey(p.Company)
		if p.Name == "" || !linkedInProfileURL(p.ProfileURL) || got == "" || !decisionTitleRe.MatchString(p.Title) {
			continue
		}
		if got == want || strings.HasPrefix(got, want+"-") || strings.HasPrefix(want, got+"-") {
			return p, true
		}
	}
	return foundPerson{}, false
}

// insertHiringSignal stores the signal as a scored lead. It skips the LLM
// scorer on purpose: the scorer treats job posts as noise, and here the job
// post is the evidence.
// It returns the new mention's id ("" when nothing was stored).
func (m *Monitor) insertHiringSignal(ctx context.Context, wsID, pid string, job jobPost, person foundPerson, plan *ai.HiringPlan) string {
	title := fmt.Sprintf("%s is hiring: %s", job.Company, job.Title)
	var b strings.Builder
	fmt.Fprintf(&b, "%s posted a job for %q", job.Company, job.Title)
	if job.Location != "" {
		fmt.Fprintf(&b, " (%s)", job.Location)
	}
	b.WriteString(" this week.")
	score, author, authorURL := hiringScoreNoPerson, job.Company, job.CompanyURL
	if person.Name != "" {
		fmt.Fprintf(&b, "\n\nWho to contact: %s, %s at %s.", person.Name, person.Title, job.Company)
		score, author, authorURL = hiringScoreWithPerson, person.Name, person.ProfileURL
	} else {
		fmt.Fprintf(&b, "\n\nNo %s found yet for this company.", plan.BuyerTitles)
	}
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
		Status:            database.MentionStatusNew,
		PlatformMetadata:  jsonBytes(map[string]any{"signal": "hiring", "company": job.Company, "company_url": job.CompanyURL, "job_title": job.Title, "contact_title": person.Title}),
		EngagementMetrics: jsonBytes(map[string]any{}),
		KeywordMatches:    []string{"hiring signal"},
		PlatformCreatedAt: created,
	}, "hiring signal")
	if alert == nil {
		return "" // duplicate, muted or too old
	}

	res := &ai.ClassifyResult{
		Intent: string(database.IntentTypeBuySignal), RelevanceScore: score, ConversionProbability: 0.3,
		Reasoning: "Company posted a matching job this week. " + plan.Why, AwarenessLevel: "problem_aware",
		IntentTags: []string{"hiring"}, Sentiment: "neutral", PostType: "other", AuthorRole: "buyer",
	}
	if _, err := m.q.UpdateMentionScoring(ctx, database.UpdateMentionScoringParams{
		ID: alert.ID, WorkspaceID: wsID,
		Intent:                database.NullIntentType{IntentType: database.IntentTypeBuySignal, Valid: true},
		ConversionProbability: pgtype.Float4{Float32: float32(res.ConversionProbability), Valid: true},
		RelevanceScore:        pgtype.Float4{Float32: float32(score), Valid: true},
		ScoringMetadata:       jsonBytes(map[string]any{"stage": "hiring_signal", "auto_scored": true, "reasoning": res.Reasoning, "author_role": "buyer", "intent_tags": res.IntentTags}),
		AwarenessLevel:        pgtype.Text{String: res.AwarenessLevel, Valid: true},
	}); err != nil {
		m.logger.Error().Err(err).Str("mention_id", alert.ID).Msg("hiring: store score")
		return alert.ID
	}
	m.storeIntentTags(ctx, wsID, alert.ID, res)
	m.qualifyAsLead(ctx, *alert, res) // creates the lead and enriches the LinkedIn profile
	return alert.ID
}

// draftOutreach saves a first message to the contact as the lead's draft, so
// the inbox and the daily digest show something ready to send. Costs the
// same as a reply draft; refunded if writing fails.
func (m *Monitor) draftOutreach(ctx context.Context, wsID, mentionID string, bp database.BrandProfile, job jobPost, person foundPerson) {
	if person.Name == "" {
		return // nobody to write to
	}
	if err := m.credits.Spend(ctx, wsID, billing.CostDraft, billing.ReasonDraft); err != nil {
		return // out of credits: the lead still stands without a draft
	}
	text, err := m.hiringPlanner.DraftOutreach(ctx, wsID, ai.Outreach{
		Product: bp.Product, Overview: bp.Overview, Positioning: bp.Positioning,
		ContactName: person.Name, ContactTitle: person.Title, Company: job.Company, JobTitle: job.Title,
	})
	if err == nil {
		_, err = m.q.CreateReply(ctx, database.CreateReplyParams{
			MentionID: mentionID, WorkspaceID: wsID, Content: text, Status: database.ReplyStatusDraft,
			TemplateStyle: pgtype.Text{String: "outreach", Valid: true},
		})
	}
	if err != nil {
		m.credits.Refund(ctx, wsID, billing.CostDraft, billing.ReasonDraft)
		m.logger.Warn().Err(err).Str("mention_id", mentionID).Msg("hiring: outreach draft failed")
	}
}
