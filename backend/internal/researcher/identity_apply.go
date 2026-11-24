package researcher

// identity: who is behind a forum handle. Tier a walks the author's public
// profiles (free, seconds). When that leaves no email or no LinkedIn, tier b
// sends a research agent (Claude Sonnet + an E2B sandbox) in the background.
// Whatever key either finds (a LinkedIn URL, a published email, a real name)
// is what makes the paid treg lookup able to match at all: before this, every
// Reddit/HN/Dev.to author reached treg as a bare handle and was skipped.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/database"
	"leadecho/internal/enrich"
	"leadecho/internal/identity"
)

// PersonAgent runs tier b (*llm.Router satisfies it).
type PersonAgent interface {
	ResearchPerson(ctx context.Context, workspaceID string, in ai.PersonResearch, tools ai.SandboxTools) (*ai.PersonFindings, error)
}

// SandboxSession is one research run's sandbox (closed when the run ends).
type SandboxSession interface {
	ai.SandboxTools
	Close()
}

// SandboxFactory opens a session; nil means research without a sandbox.
type SandboxFactory func() SandboxSession

const (
	agentTimeout = 8 * time.Minute
	// minNameLen: a "name" shorter than this ("J") can't confirm a LinkedIn.
	minNameLen = 5
)

// agentSlots bounds concurrent research agents (each holds a sandbox).
var agentSlots = make(chan struct{}, 3)

// SetIdentity enables tier a.
func (s *Service) SetIdentity(r *identity.Resolver) { s.identity = r }

// SetPersonAgent enables tier b. dailyCap is per workspace, globalCap across
// all of them (either 0 = off).
func (s *Service) SetPersonAgent(a PersonAgent, sandbox SandboxFactory, dailyCap, globalCap int) {
	if a == nil || dailyCap <= 0 || globalCap <= 0 {
		return
	}
	s.agent, s.sandbox, s.agentCap, s.agentGlobalCap = a, sandbox, dailyCap, globalCap
}

// resolveIdentity runs tier a for a lead's author and saves it. nil when the
// platform has no public profile to start from (LinkedIn goes to treg as is).
func (s *Service) resolveIdentity(ctx context.Context, ws string, lead database.Lead, personID, platform, handle, profileURL string) *identity.Profile {
	if s.identity == nil || handle == "" {
		return nil
	}
	if prev := storedIdentity(lead.Metadata); prev != nil {
		return prev
	}
	seed := identity.Seed{Platform: platform, Handle: handle, ProfileURL: profileURL}
	prof, err := s.identity.Resolve(ctx, seed)
	if err != nil {
		if !errors.Is(err, identity.ErrUnsupported) { // LinkedIn authors: nothing to try again
			s.logger.Debug().Err(err).Str("lead_id", lead.ID).Msg("identity: resolve failed")
			s.markAttempt(ctx, ws, lead.ID)
		}
		return nil
	}
	if prof.Incomplete && !prof.HasShown() {
		// A source failed (often GitHub's rate limit): use nothing, store
		// nothing, so the backfill tries again in a week.
		s.markAttempt(ctx, ws, lead.ID)
		return nil
	}
	if err := s.saveIdentity(ctx, ws, lead.ID, personID, prof); err != nil {
		s.logger.Warn().Err(err).Str("lead_id", lead.ID).Msg("identity: save")
	}
	return prof
}

// markAttempt keeps the backfill from queueing the same lead every run.
func (s *Service) markAttempt(ctx context.Context, ws, leadID string) {
	patch, _ := json.Marshal(map[string]any{"identity_attempt_at": time.Now().UTC()})
	_ = s.q.MergeLeadMetadata(ctx, ws, leadID, patch)
}

func storedIdentity(meta []byte) *identity.Profile {
	var m struct {
		Identity *identity.Profile `json:"identity"`
	}
	if json.Unmarshal(meta, &m) != nil {
		return nil
	}
	return m.Identity
}

// saveIdentity stores the profile on the lead and the person and adds every
// shown link as a person identity. Fields on the person and the lead are
// only filled where empty: what treg verified or a user typed wins.
func (s *Service) saveIdentity(ctx context.Context, ws, leadID, personID string, p *identity.Profile) error {
	patch, _ := json.Marshal(map[string]any{"identity": p})
	if err := s.q.MergeLeadMetadata(ctx, ws, leadID, patch); err != nil {
		return fmt.Errorf("lead metadata: %w", err)
	}
	person, err := s.q.GetPerson(ctx, database.GetPersonParams{ID: personID, WorkspaceID: ws})
	if err != nil {
		return fmt.Errorf("get person: %w", err)
	}
	conf := person.Confidence
	if _, ok := p.Best(identity.KindLinkedIn); ok || p.Email() != "" {
		conf = max(conf, 0.85)
	}
	if _, err := s.q.UpdatePerson(ctx, database.UpdatePersonParams{
		ID: personID, WorkspaceID: ws,
		DisplayName: fillName(person.DisplayName, p.Name),
		Bio:         fillIf(person.Bio, firstNonEmpty(p.Summary, p.Bio)),
		Company:     fillIf(person.Company, p.Company),
		Location:    fillIf(person.Location, p.Location),
		Confidence:  conf, Metadata: mergeMeta(person.Metadata, "identity", p),
	}); err != nil {
		return fmt.Errorf("update person: %w", err)
	}
	for _, l := range p.Links {
		if l.Confidence < identity.Shown {
			continue
		}
		// Never take over a row that already belongs to someone else (the
		// upsert would move it): it may be another lead's own author.
		if cur, err := s.q.GetPersonIdentity(ctx, database.GetPersonIdentityParams{
			WorkspaceID: ws, Platform: l.Kind, Handle: identityHandle(l),
		}); err == nil && cur.PersonID != personID {
			continue
		}
		meta, _ := json.Marshal(map[string]string{"evidence": l.Evidence, "method": l.Method})
		if _, err := s.q.UpsertPersonIdentity(ctx, database.UpsertPersonIdentityParams{
			PersonID: personID, WorkspaceID: ws, Platform: l.Kind, Handle: identityHandle(l),
			ProfileUrl: txtIf(l.URL), Confidence: float32(l.Confidence), Source: "identity_" + p.Tier, Metadata: meta,
		}); err != nil {
			return fmt.Errorf("upsert identity: %w", err)
		}
	}
	// The email goes on the lead through enrichment, which verifies it (or
	// marks it "published"); only without treg is it set here directly.
	email := ""
	if !s.enricher.Enabled() {
		email = p.Email()
	}
	if err := s.q.FillLeadContact(ctx, ws, leadID, p.Name, email, p.Company); err != nil {
		return fmt.Errorf("fill lead contact: %w", err)
	}
	return nil
}

// fillIf sets v only when the current value is empty.
func fillIf(cur pgtype.Text, v string) pgtype.Text {
	if cur.Valid && strings.TrimSpace(cur.String) != "" {
		return pgtype.Text{}
	}
	return txtIf(v)
}

// fillName replaces a placeholder display name (the handle, "Unknown") with
// a real one, never a name already found.
func fillName(cur pgtype.Text, v string) pgtype.Text {
	if cur.Valid && strings.Contains(strings.TrimSpace(cur.String), " ") {
		return pgtype.Text{}
	}
	return txtIf(v)
}

func identityHandle(l identity.Link) string {
	if l.Handle != "" {
		return l.Handle
	}
	return l.URL
}

// withIdentity adds what the walk found to the treg subject: the keys a
// person lookup can match on.
func withIdentity(sub enrich.Subject, p *identity.Profile) enrich.Subject {
	if p == nil {
		return sub
	}
	if li, ok := p.Best(identity.KindLinkedIn); ok && !strings.Contains(strings.ToLower(sub.ProfileURL), "linkedin.com/in/") {
		sub.ProfileURL = li.URL
	}
	if e, ok := p.Best(identity.KindEmail); ok && sub.Email == "" {
		sub.Email, sub.EmailSource = e.Handle, e.Evidence
	}
	sub.Name = firstNonEmpty(sub.Name, p.Name)
	sub.Company = firstNonEmpty(sub.Company, p.Company)
	return sub
}

// lookupable: the paid lookup has something to match on. A forum author
// with no LinkedIn URL or email would only skip every step and still use
// one of the workspace's daily enrichments, which then blocks the authors
// the research agent does find a key for. LinkedIn and X authors are
// matched by their handle (X through its profile), so they always go.
func lookupable(sub enrich.Subject) bool {
	switch strings.ToLower(sub.Platform) {
	case "hackernews", "devto", "bluesky", "reddit", "indiehackers", "lobsters":
		return sub.Email != "" || strings.Contains(strings.ToLower(sub.ProfileURL), "linkedin.com/in/")
	}
	return true
}

// needsEnrichment: never enriched, or enriched as a bare handle (every paid
// step skipped) and now there is a key to match on.
func needsEnrichment(meta []byte, sub enrich.Subject) bool {
	var m struct {
		Enrichment *enrich.Result `json:"enrichment"`
	}
	if json.Unmarshal(meta, &m) != nil || m.Enrichment == nil {
		return true
	}
	hasKey := sub.Email != "" || strings.Contains(strings.ToLower(sub.ProfileURL), "linkedin.com/in/")
	prev := m.Enrichment
	return hasKey && prev.Steps[enrich.ToolPerson] == enrich.StepSkipped && prev.Email == ""
}

// ---- tier b ------------------------------------------------------------

// agentDue reports whether the research agent should run for this profile.
func agentDue(p *identity.Profile) bool {
	return p != nil && p.Tier != "b" && !p.Complete()
}

// deepDue: we know who they are (a real name and a verified link) and their
// public footprint (every profile, their posts, talks, projects) was never
// mapped. One deep run per person.
func deepDue(p *identity.Profile) bool {
	return p.Identified() && p.DeepAt.IsZero()
}

type dayCounter struct {
	mu   sync.Mutex
	day  string
	runs map[string]int
}

// ponytail: per-process counters, reset by a restart (one API instance
// today). Move to Redis if the API is scaled out or deploys get frequent.
var agentRuns = &dayCounter{runs: map[string]int{}}

// takeBoth takes one run from the workspace's cap and the global cap, or
// neither.
func (c *dayCounter) takeBoth(ws string, limit, global int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.roll()
	total := 0
	for _, n := range c.runs {
		total += n
	}
	if total >= global || c.runs[ws] >= limit {
		return false
	}
	c.runs[ws]++
	return true
}

func (c *dayCounter) roll() {
	if d := time.Now().UTC().Format("2006-01-02"); d != c.day {
		c.day, c.runs = d, map[string]int{}
	}
}

func (c *dayCounter) take(ws string, limit int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.roll()
	if c.runs[ws] >= limit {
		return false
	}
	c.runs[ws]++
	return true
}

// inflightAuthors: one research run per author at a time, across
// workspaces (the result is cached for all of them when it finishes).
var inflightAuthors sync.Map

// researchAsync runs tier b in the background, then the paid lookup again
// with whatever new key it found.
func (s *Service) researchAsync(ws, leadID, personID string, mention database.Mention, platform, handle, profileURL string, prof *identity.Profile) {
	if s.agent == nil || (!agentDue(prof) && !deepDue(prof)) {
		return
	}
	author := strings.ToLower(platform + ":" + handle)
	if _, busy := inflightAuthors.LoadOrStore(author, struct{}{}); busy {
		return
	}
	if !agentRuns.takeBoth(ws, s.agentCap, s.agentGlobalCap) {
		inflightAuthors.Delete(author)
		return
	}
	go func() {
		defer inflightAuthors.Delete(author)
		defer func() {
			if r := recover(); r != nil { // a bug here must not take the API down
				s.logger.Error().Interface("panic", r).Str("lead_id", leadID).Msg("identity: research agent panicked")
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), agentTimeout)
		defer cancel()
		select { // waiting for a slot counts against the run's time
		case agentSlots <- struct{}{}:
			defer func() { <-agentSlots }()
		case <-ctx.Done():
			return
		}
		if err := s.research(ctx, ws, leadID, personID, mention, platform, handle, profileURL, prof); err != nil {
			s.logger.Warn().Err(err).Str("lead_id", leadID).Msg("identity: research agent")
		}
	}()
}

func (s *Service) research(ctx context.Context, ws, leadID, personID string, mention database.Mention, platform, handle, profileURL string, prof *identity.Profile) error {
	in := ai.PersonResearch{Platform: platform, Handle: handle, ProfileURL: profileURL, Name: prof.Name, Notes: prof.Notes, Deep: deepDue(prof),
		Post: strings.TrimSpace(mention.Title.String + "\n" + mention.Content)}
	for _, l := range prof.Links {
		if l.Confidence >= 0.5 {
			in.Known = append(in.Known, fmt.Sprintf("%s %s (%.2f, from %s)", l.Kind, l.URL, l.Confidence, firstNonEmpty(l.Evidence, "their profile")))
		}
	}
	var tools ai.SandboxTools
	if s.sandbox != nil {
		if sb := s.sandbox(); sb != nil {
			defer sb.Close()
			tools = sb
		}
	}
	found, err := s.agent.ResearchPerson(ctx, ws, in, tools)
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	next := identity.MergeAgent(prof, found) // tier "b" even on failure: one attempt per author
	if in.Deep {
		next.DeepAt = time.Now().UTC() // one deep run per person, even if it found little
	}
	next = s.confirmLinkedIn(saveCtx, ws, next)
	s.identity.Store(saveCtx, identity.Seed{Platform: platform, Handle: handle}, next)
	if serr := s.saveIdentity(saveCtx, ws, leadID, personID, next); serr != nil {
		return fmt.Errorf("save: %w", serr)
	}
	if fresh, gerr := s.q.GetLead(saveCtx, database.GetLeadParams{ID: leadID, WorkspaceID: ws}); gerr == nil {
		s.enrichWithTreg(saveCtx, ws, fresh, personID, platform, handle, profileURL, next)
	}
	s.dossierAsync(ws, personID, platform, handle) // with what the agent found
	return err
}

// confirmLinkedIn checks the best LinkedIn candidate the agent found by
// searching (below Shown) with one person lookup per run. It is kept only
// when the profile's name is the person's name AND a second thing agrees
// (the company, or the slug spells the surname): a common name alone would
// put a stranger's job and email on the lead. The full enrichment that
// follows asks the same question and is served from treg's cache.
func (s *Service) confirmLinkedIn(ctx context.Context, ws string, p *identity.Profile) *identity.Profile {
	if _, ok := p.Best(identity.KindLinkedIn); ok || len([]rune(p.Name)) < minNameLen || !s.enricher.Enabled() {
		return p
	}
	var best identity.Link
	for _, l := range p.Links {
		if l.Kind == identity.KindLinkedIn && l.Confidence >= 0.6 && l.Confidence > best.Confidence {
			best = l
		}
	}
	if best.URL == "" {
		return p
	}
	m, err := s.enricher.PersonLookup(ctx, ws, best.URL)
	if err != nil || !sameName(m.Name, p.Name) {
		return p
	}
	if sameCompany(m.Company, p.Company) || slugHasSurname(best.Handle, p.Name) {
		return identity.Confirm(p, best.URL, 0.9, "name_confirmed")
	}
	return p
}

func sameCompany(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	return len(a) >= 3 && len(b) >= 3 && (strings.Contains(a, b) || strings.Contains(b, a))
}

func slugHasSurname(slug, name string) bool {
	f := strings.Fields(strings.ToLower(name))
	return len(f) >= 2 && len([]rune(f[len(f)-1])) >= 3 && strings.Contains(strings.ToLower(slug), f[len(f)-1])
}

// sameName: "Jane Doe" and "jane doe" match; "Jane Q. Doe" matches "Jane Doe"
// (same first and last word).
func sameName(a, b string) bool {
	fa, fb := strings.Fields(strings.ToLower(a)), strings.Fields(strings.ToLower(b))
	if len(fa) < 2 || len(fb) < 2 {
		return false
	}
	return fa[0] == fb[0] && fa[len(fa)-1] == fb[len(fb)-1]
}
