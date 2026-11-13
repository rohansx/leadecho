package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/net/publicsuffix"

	"leadecho/internal/ai"
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
	tregCompanySearchTool = "treg.companies.search"

	prospectSettingsKey = "prospecting"
	prospectInterval    = 24 * time.Hour
	// Rows per company search: the provider bills per row, so this is the
	// price dial. Most rows are dropped by the fit check.
	prospectSearchLimit = 15
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

func (m *Monitor) scanProspects(ctx context.Context, wsID string) {
	if m.prospectJudge == nil || m.hiringPlanner == nil || !m.treg.Enabled() || m.credits.PaidPlatformsLocked(ctx, wsID) || m.credits.PaidDataPaused(ctx) {
		return
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
	queries := segmentQueries(seg)
	query := queries[(st.Round/len(segments))%len(queries)]
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
	candidates, err := m.searchCompanies(ctx, wsID, query, seg.Country)
	if err != nil {
		return // out of credits, paused or provider down: the round still advances
	}
	candidates = newProspects(candidates, seen, bp.SourceUrl, bp.Competitors)
	if len(candidates) == 0 {
		m.logger.Info().Str("segment", seg.Name).Str("workspace_id", wsID).Msg("prospects: no new companies")
		return
	}
	fits, err := m.prospectJudge.FitProspects(ctx, wsID, ai.ProspectCheck{
		Product: bp.Product, Overview: bp.Overview, Segment: seg, Competitors: bp.Competitors, Candidates: candidates,
	})
	if err != nil {
		m.logger.Warn().Err(err).Str("workspace_id", wsID).Msg("prospects: fit check failed")
		return // the companies stay unseen: judged on a later run
	}
	fitting := map[int]bool{}
	for _, f := range fits {
		fitting[f.Index] = true
	}
	for i, c := range candidates {
		if !fitting[i] {
			remember(c.Domain) // judged not to fit: never paid for again
		}
	}
	// A fitting company is remembered once its people search has run, so
	// one left over by today's cap or a failed search is found again later.

	capContacts := starterProspectContacts
	if bs, err := m.q.GetBillingState(ctx, wsID); err == nil && bs.Plan == "pro" {
		capContacts = maxProspectContactsPerRun
	}
	created, lookups := 0, 0
	for _, fit := range fits {
		if lookups >= capContacts || ctx.Err() != nil {
			break
		}
		c := candidates[fit.Index]
		pid := prospectPlatformID(c.Domain)
		exists, err := m.q.MentionExists(ctx, wsID, string(database.PlatformTypeExa), pid)
		if err != nil {
			continue
		}
		if exists {
			remember(c.Domain)
			continue
		}
		lookups++
		done, stop := m.prospectCompany(ctx, wsID, bp, seg, c, fit, pid, remember)
		if done {
			created++
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
func (m *Monitor) prospectCompany(ctx context.Context, wsID string, bp database.BrandProfile, seg ai.CustomerSegment, c ai.ProspectCandidate, fit ai.ProspectFit, pid string, remember func(string)) (created, stop bool) {
	job := jobPost{Company: c.Name, CompanyURL: "https://" + c.Domain} // no job: the buyer query names the titles
	candidates, err := m.hiringBuyers(ctx, wsID, seg.BuyerTitles, job, c.Employees)
	if err != nil {
		return false, true
	}
	co := hiringCompany{size: c.Employees, domain: c.Domain}
	person, contact, check, capped := chooseContact(candidates,
		func(p foundPerson) (*enrich.Result, bool) { return m.verifyContact(ctx, wsID, job, p) },
		func(p foundPerson, res *enrich.Result) contactCheck { return confirmContact(c.Name, co, 0, p, res) })
	if capped && person.Name == "" {
		return false, true // today's contact lookups ran out before anyone was checked: retry another day
	}
	remember(c.Domain)
	if !check.confirmed {
		// Without a confirmed buyer a fitting company is only a name on a
		// list: not stored, and (already judged) not looked up again.
		return false, capped
	}
	brief := m.researchLead(ctx, wsID, ai.CompanyResearch{
		Product: bp.Product, Overview: bp.Overview,
		Company: c.Name, CompanyURL: job.CompanyURL, ContactTitle: person.Title,
	})
	if ctx.Err() != nil {
		m.refundResearch(ctx, wsID, brief)
		return false, true
	}
	mentionID := m.insertProspect(ctx, wsID, pid, seg, c, fit, person, contact, brief)
	if mentionID == "" {
		m.refundResearch(ctx, wsID, brief)
		return false, capped
	}
	m.draftOutreach(ctx, wsID, mentionID, ai.Outreach{
		Product: bp.Product, Overview: bp.Overview, Positioning: bp.Positioning,
		ContactName: person.Name, ContactTitle: person.Title, Company: c.Name,
		Research: brief, SegmentPain: seg.Pain,
	})
	return true, capped
}

// segmentQueries are the wordings of a segment's company search, one per
// run in turn. The same query returns the same companies, all seen after
// the first run, so later runs ask with one criterion at a time.
// ponytail: a fixed list of wordings; once they are used up a segment finds
// little. Next step: companies.similar seeded with the domains of confirmed
// prospects.
func segmentQueries(s ai.CustomerSegment) []string {
	out := []string{s.Name + ": " + strings.Join(s.Criteria, "; ")}
	for _, c := range s.Criteria {
		out = append(out, s.Name+": "+c)
	}
	return out
}

func prospectPlatformID(domain string) string { return "prospect-" + domain }

// searchCompanies runs one company search. An error means the search
// itself failed; no companies with a nil error means it found none.
func (m *Monitor) searchCompanies(ctx context.Context, wsID, query, country string) ([]ai.ProspectCandidate, error) {
	body := map[string]any{"q": query, "limit": prospectSearchLimit}
	if country != "" {
		body["country"] = country
	}
	resp, err := m.treg.Call(ctx, treg.Request{WorkspaceID: wsID, Tool: tregCompanySearchTool, Body: body})
	if err != nil {
		if !treg.IsOperatorError(err) {
			m.logger.Debug().Err(err).Str("workspace_id", wsID).Msg("prospects: company search failed")
		}
		return nil, err
	}
	return parseCompanies(resp.Body), nil
}

// parseCompanies reads {output:{companies:[...]}} (or unwrapped). Rows are
// provider-native, so each field is taken from whichever key the provider
// uses. A row without a usable website domain is dropped: the domain is
// what the contact's employer is checked against.
func parseCompanies(body []byte) []ai.ProspectCandidate {
	var v struct {
		Output *struct {
			Companies []map[string]any `json:"companies"`
		} `json:"output"`
		Companies []map[string]any `json:"companies"`
	}
	if json.Unmarshal(body, &v) != nil {
		return nil
	}
	rows := v.Companies
	if v.Output != nil {
		rows = v.Output.Companies
	}
	var out []ai.ProspectCandidate
	byDomain := map[string]bool{}
	for _, r := range rows {
		domain := companyDomain(firstString(r, "domain", "website", "website_url", "websiteUrl", "url", "homepage", "company_domain"))
		name := firstString(r, "name", "company_name", "companyName", "title")
		if domain == "" || name == "" || byDomain[domain] {
			continue
		}
		byDomain[domain] = true
		out = append(out, ai.ProspectCandidate{
			Name:        truncateRunes(name, maxProspectName), // provider text: it reaches prompts and the lead's title
			Domain:      domain,
			Description: firstString(r, "description", "short_description", "shortDescription", "summary", "about", "tagline"),
			Employees:   enrich.ParseSize(firstValue(r, "employees", "employee_count", "employeeCount", "headcount", "size", "company_size", "estimated_num_employees")),
			Country:     firstString(r, "country", "country_code", "countryCode", "hq_country"),
		})
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
	if contact.EmailStatus == "catchall" {
		b.WriteString(" Their email could not be verified (the company's mail server accepts any address), so it may bounce.")
	}
	b.WriteString(researchNote(brief))
	if seg.Pain != "" {
		b.WriteString("\n\nWhat this segment struggles with: " + seg.Pain)
	}
	companyURL := "https://" + c.Domain
	meta := map[string]any{"signal": "prospect", "company": c.Name, "company_url": companyURL, "segment": seg.Name,
		"fit_reason": fit.Reason, "contact_title": contactTitle, "confirmed": true}
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
	score := hiringLeadScore(contact) - prospectScoreDiscount
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
		tag: "prospect", score: score, intent: database.IntentTypeGeneral})
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
