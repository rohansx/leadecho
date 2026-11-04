// Package enrich turns a qualified lead's author into a contact through treg:
// person -> company -> work email -> deliverability check (plan P1 #11).
// Each step is tolerant of misses and errors; a lead is never blocked on it.
package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/database"
	"leadecho/internal/treg"
)

// Catalog tools (routed ones answer {output:{...}}; filtrat answers verbatim).
const (
	ToolPerson      = "treg.people.enrich"
	ToolCompany     = "treg.companies.enrich"
	ToolEmailFind   = "treg.people.email.find"
	ToolEmailVerify = "filtrat.people.email.verify"
)

// Step outcomes recorded in Result.Steps.
const (
	StepHit     = "hit"
	StepMiss    = "miss"
	StepError   = "error"
	StepSkipped = "skipped"
)

var (
	ErrDisabled = errors.New("enrich: treg not configured")
	ErrDailyCap = errors.New("enrich: workspace daily enrichment cap reached")
)

// Subject is what we know about a lead's author before enrichment.
type Subject struct {
	Name, Handle, Platform, ProfileURL, Company, Email string
}

// Result is stored as leads.metadata.enrichment and persons.metadata.enrichment.
type Result struct {
	FullName      string            `json:"full_name,omitempty"`
	Title         string            `json:"title,omitempty"`
	LinkedInURL   string            `json:"linkedin_url,omitempty"`
	Location      string            `json:"location,omitempty"`
	Company       string            `json:"company,omitempty"`
	CompanyDomain string            `json:"company_domain,omitempty"`
	Industry      string            `json:"industry,omitempty"`
	Employees     string            `json:"employees,omitempty"`
	CompanySize   int               `json:"company_size,omitempty"`
	Email         string            `json:"email,omitempty"`
	EmailStatus   string            `json:"email_status,omitempty"` // verified|invalid|catchall|risky|unknown
	Steps         map[string]string `json:"steps"`
	EnrichedAt    time.Time         `json:"enriched_at"`
}

// Found reports whether any step produced data.
func (r *Result) Found() bool {
	return r.FullName != "" || r.Company != "" || r.Email != ""
}

// Caller is the slice of *treg.Client the pipeline uses.
type Caller interface {
	Enabled() bool
	Call(ctx context.Context, req treg.Request) (*treg.Response, error)
}

// Pipeline runs the four steps; it knows nothing about caps or storage.
type Pipeline struct {
	c      Caller
	logger zerolog.Logger
}

func NewPipeline(c *treg.Client, logger zerolog.Logger) *Pipeline {
	p := &Pipeline{logger: logger}
	if c.Enabled() { // avoid a typed-nil interface
		p.c = c
	}
	return p
}

func (p *Pipeline) Run(ctx context.Context, ws string, s Subject) (*Result, error) {
	if p.c == nil || !p.c.Enabled() {
		return nil, ErrDisabled
	}
	r := &Result{Steps: map[string]string{}, EnrichedAt: time.Now().UTC(), Email: s.Email}
	stop := false // an operator error (balance, team cap) ends the run: later steps would fail too
	step := func(tool string, body map[string]any, apply func(map[string]any) bool) {
		if stop || body == nil {
			r.Steps[tool] = StepSkipped
			return
		}
		out, err := p.call(ctx, ws, tool, body)
		switch {
		case err != nil:
			r.Steps[tool] = StepError
			stop = treg.IsOperatorError(err)
		case apply(out):
			r.Steps[tool] = StepHit
		default:
			r.Steps[tool] = StepMiss
		}
	}

	step(ToolPerson, personBody(s), r.applyPerson)
	if r.Company == "" {
		r.Company = strings.TrimSpace(s.Company)
	}
	step(ToolCompany, r.companyBody(s), r.applyCompany)
	if s.Email != "" {
		r.Steps[ToolEmailFind] = StepSkipped
	} else {
		step(ToolEmailFind, r.emailFindBody(s), r.applyEmail)
	}
	verify := map[string]any(nil)
	if r.Email != "" {
		verify = map[string]any{"email": r.Email, "resolve_catchall": false}
	}
	step(ToolEmailVerify, verify, r.applyVerify)
	return r, nil
}

func (p *Pipeline) call(ctx context.Context, ws, tool string, body map[string]any) (map[string]any, error) {
	resp, err := p.c.Call(ctx, treg.Request{WorkspaceID: ws, Tool: tool, Body: body})
	if err != nil {
		if !treg.IsOperatorError(err) { // operator errors are alerted by the client
			p.logger.Debug().Err(err).Str("tool", tool).Str("workspace_id", ws).Msg("enrich: step failed")
		}
		return nil, err
	}
	var v map[string]any
	if json.Unmarshal(resp.Body, &v) != nil {
		return map[string]any{}, nil
	}
	if o, ok := v["output"].(map[string]any); ok {
		return o, nil
	}
	return v, nil
}

func isLinkedInProfile(u string) bool {
	return strings.Contains(strings.ToLower(u), "linkedin.com/in/")
}

var slugRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,99}$`)

func personBody(s Subject) map[string]any {
	switch {
	case isLinkedInProfile(s.ProfileURL):
		return map[string]any{"linkedin_url": s.ProfileURL}
	case s.Email != "":
		return map[string]any{"email": s.Email}
	case s.Platform == "linkedin" && slugRe.MatchString(s.Handle):
		return map[string]any{"linkedin_handle": s.Handle}
	}
	return nil // Reddit/HN/X handles are pseudonymous: nothing treg can match on
}

func (r *Result) companyBody(s Subject) map[string]any {
	switch {
	case r.CompanyDomain != "":
		return map[string]any{"domain": r.CompanyDomain}
	case r.Company != "":
		return map[string]any{"name": r.Company}
	case s.Email != "":
		return map[string]any{"email": s.Email}
	}
	return nil
}

func (r *Result) emailFindBody(s Subject) map[string]any {
	name := firstNonEmpty(r.FullName, s.Name)
	switch {
	case r.LinkedInURL != "":
		return map[string]any{"linkedin_url": r.LinkedInURL}
	case isLinkedInProfile(s.ProfileURL):
		return map[string]any{"linkedin_url": s.ProfileURL}
	case r.CompanyDomain != "" && strings.Contains(strings.TrimSpace(name), " "):
		return map[string]any{"domain": r.CompanyDomain, "full_name": name}
	case s.Platform == "linkedin" && slugRe.MatchString(s.Handle):
		return map[string]any{"linkedin_handle": s.Handle}
	}
	return nil
}

func (r *Result) applyPerson(o map[string]any) bool {
	if r.FullName = str(o, "full_name"); r.FullName == "" {
		return false
	}
	r.Title = str(o, "title")
	r.Company = str(o, "company")
	r.CompanyDomain = str(o, "company_domain")
	r.LinkedInURL = str(o, "linkedin_url")
	r.Location = str(o, "location")
	return true
}

func (r *Result) applyCompany(o map[string]any) bool {
	name := str(o, "name")
	if name == "" {
		return false
	}
	r.Company = name
	r.CompanyDomain = firstNonEmpty(str(o, "domain"), r.CompanyDomain)
	r.Industry = str(o, "industry")
	r.Employees = str(o, "employees")
	r.CompanySize = parseSize(o["employees"])
	if r.Location == "" {
		r.Location = str(o, "location")
	}
	return true
}

func (r *Result) applyEmail(o map[string]any) bool {
	r.Email = str(o, "email")
	return r.Email != ""
}

func (r *Result) applyVerify(o map[string]any) bool {
	r.EmailStatus = str(o, "status")
	return r.EmailStatus != ""
}

func str(m map[string]any, k string) string {
	switch v := m[k].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

var sizeRe = regexp.MustCompile(`(\d[\d,]*)\s*([kK])?`)

// parseSize reads a headcount "as the provider states it" (250, "51-200",
// "1,001-5,000", "10K+") as its lower bound. 0 = unknown.
func parseSize(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case string:
		m := sizeRe.FindStringSubmatch(t)
		if m == nil {
			return 0
		}
		n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
		if err != nil {
			return 0
		}
		if m[2] != "" {
			n *= 1000
		}
		return n
	}
	return 0
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ---- caps -----------------------------------------------------------------

// Counter counts leads enriched since a time (*database.Queries satisfies it).
type Counter interface {
	CountEnrichedLeadsSince(ctx context.Context, arg database.CountEnrichedLeadsSinceParams) (int64, error)
}

type budgeter interface {
	SetCustomerDailyBudget(ctx context.Context, workspaceID string, capMicro int64) error
}

// Enricher is the pipeline behind a per-workspace daily lead cap (ours) and,
// when budgetMicro > 0, a treg customer daily budget (theirs, set once per
// workspace per process). A nil *Enricher is valid and disabled.
type Enricher struct {
	pipeline    *Pipeline
	counter     Counter
	dailyCap    int
	budget      budgeter
	budgetMicro int64
	budgeted    sync.Map // workspaceID -> struct{}
	logger      zerolog.Logger

	// inflight counts enrichments that passed the cap but are not stored yet,
	// so concurrent calls can't all read the same stored count and overrun it.
	// ponytail: per process; replicas can still each spend up to the cap.
	mu       sync.Mutex
	inflight map[string]int
}

// New returns nil when treg is off or dailyCap <= 0 (feature off).
func New(c *treg.Client, counter Counter, dailyCap int, budgetMicro int64, logger zerolog.Logger) *Enricher {
	if !c.Enabled() || dailyCap <= 0 {
		return nil
	}
	return &Enricher{
		pipeline: NewPipeline(c, logger), counter: counter, dailyCap: dailyCap,
		budget: c, budgetMicro: budgetMicro, logger: logger,
	}
}

func (e *Enricher) Enabled() bool { return e != nil }

func (e *Enricher) Enrich(ctx context.Context, ws string, s Subject) (*Result, error) {
	if !e.Enabled() {
		return nil, ErrDisabled
	}
	release, err := e.reserve(ctx, ws)
	if err != nil {
		return nil, err
	}
	defer release()
	e.ensureBudget(ctx, ws)
	return e.pipeline.Run(ctx, ws, s)
}

// reserve atomically checks stored + in-flight enrichments against the cap.
func (e *Enricher) reserve(ctx context.Context, ws string) (func(), error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	day := time.Now().UTC().Truncate(24 * time.Hour)
	n, err := e.counter.CountEnrichedLeadsSince(ctx, database.CountEnrichedLeadsSinceParams{WorkspaceID: ws, Since: day})
	if err != nil {
		return nil, err
	}
	if e.inflight == nil {
		e.inflight = map[string]int{}
	}
	if n+int64(e.inflight[ws]) >= int64(e.dailyCap) {
		return nil, ErrDailyCap
	}
	e.inflight[ws]++
	return func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.inflight[ws]--
	}, nil
}

func (e *Enricher) ensureBudget(ctx context.Context, ws string) {
	if e.budgetMicro <= 0 || e.budget == nil {
		return
	}
	if _, done := e.budgeted.LoadOrStore(ws, struct{}{}); done {
		return
	}
	if err := e.budget.SetCustomerDailyBudget(ctx, ws, e.budgetMicro); err != nil {
		e.budgeted.Delete(ws) // retry on the next enrichment
		e.logger.Warn().Err(err).Str("workspace_id", ws).Msg("enrich: could not set treg customer budget")
	}
}
