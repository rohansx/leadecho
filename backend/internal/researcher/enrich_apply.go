package researcher

// enrich (P1 #11): treg person/company/email enrichment for qualified leads,
// stored on the lead, the person and a LinkedIn identity, then synced to CRMs.

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/crmsync"
	"leadecho/internal/database"
	"leadecho/internal/enrich"
)

// CRMEmitter is the slice of *crmsync.Dispatcher used after enrichment.
type CRMEmitter interface {
	Emit(wsID string, ev crmsync.Event)
}

// SetEnricher enables treg enrichment (nil = off, e.g. TREG_TOKEN unset).
func (s *Service) SetEnricher(e *enrich.Enricher) { s.enricher = e }

// SetCRM pushes enriched leads to the workspace's CRM sinks.
func (s *Service) SetCRM(c *crmsync.Dispatcher) {
	if c != nil { // avoid a typed-nil interface
		s.crm = c
	}
}

// enrichWithTreg runs the paid pipeline once per lead. Every failure is
// logged and swallowed: the lead stays usable without enrichment.
func (s *Service) enrichWithTreg(ctx context.Context, ws string, lead database.Lead, personID, platform, handle, profileURL string) *enrich.Result {
	if !s.enricher.Enabled() || alreadyEnriched(lead.Metadata) {
		return nil
	}
	res, err := s.enricher.Enrich(ctx, ws, enrichSubject(lead, platform, handle, profileURL))
	if err != nil {
		if !errors.Is(err, enrich.ErrDailyCap) {
			s.logger.Warn().Err(err).Str("lead_id", lead.ID).Msg("treg enrichment failed")
		}
		return nil
	}
	if err := s.saveEnrichment(ctx, ws, lead, personID, res); err != nil {
		s.logger.Error().Err(err).Str("lead_id", lead.ID).Msg("save enrichment")
		return res
	}
	if s.crm != nil && res.Found() {
		s.crm.Emit(ws, crmsync.Event{Type: crmsync.EventLeadEnriched, LeadID: lead.ID})
	}
	return res
}

func (s *Service) saveEnrichment(ctx context.Context, ws string, lead database.Lead, personID string, res *enrich.Result) error {
	// Marker first: a crash after a paid run must not pay again.
	patch, _ := json.Marshal(map[string]any{"enrichment": res})
	if err := s.q.MergeLeadMetadata(ctx, ws, lead.ID, patch); err != nil {
		return err
	}
	if !res.Found() {
		return nil
	}
	if _, err := s.q.UpdateLead(ctx, leadUpdateFromEnrichment(lead, res)); err != nil {
		return err
	}
	person, err := s.q.GetPerson(ctx, database.GetPersonParams{ID: personID, WorkspaceID: ws}) // fresh: GitHub may have just written metadata
	if err != nil {
		return err
	}
	if _, err := s.q.UpdatePerson(ctx, database.UpdatePersonParams{
		ID:          person.ID,
		WorkspaceID: ws,
		DisplayName: txtIf(res.FullName),
		Company:     txtIf(res.Company),
		Location:    txtIf(res.Location),
		Confidence:  max(person.Confidence, 0.8),
		Metadata:    mergeMeta(person.Metadata, "enrichment", res),
	}); err != nil {
		return err
	}
	if slug := linkedInSlug(res.LinkedInURL); slug != "" {
		meta, _ := json.Marshal(map[string]string{"title": res.Title})
		if _, err := s.q.UpsertPersonIdentity(ctx, database.UpsertPersonIdentityParams{
			PersonID: person.ID, WorkspaceID: ws, Platform: "linkedin", Handle: slug,
			ProfileUrl: txtIf(res.LinkedInURL), Confidence: 0.8, Source: "treg", Metadata: meta,
		}); err != nil {
			return err
		}
	}
	return nil
}

// leadUpdateFromEnrichment fills only empty contact fields, so nothing a
// human typed is overwritten. Undeliverable emails are not used as contact.
func leadUpdateFromEnrichment(lead database.Lead, r *enrich.Result) database.UpdateLeadParams {
	p := database.UpdateLeadParams{ID: lead.ID, WorkspaceID: lead.WorkspaceID, Stage: lead.Stage}
	if !lead.ContactName.Valid || lead.ContactName.String == "" {
		p.ContactName = txtIf(r.FullName)
	}
	if (!lead.ContactEmail.Valid || lead.ContactEmail.String == "") && r.EmailStatus != "invalid" {
		p.ContactEmail = txtIf(r.Email)
	}
	if !lead.Company.Valid || lead.Company.String == "" {
		p.Company = txtIf(r.Company)
	}
	return p
}

func enrichSubject(lead database.Lead, platform, handle, profileURL string) enrich.Subject {
	return enrich.Subject{
		Name: lead.ContactName.String, Handle: handle, Platform: platform,
		ProfileURL: profileURL, Company: lead.Company.String, Email: lead.ContactEmail.String,
	}
}

func alreadyEnriched(meta []byte) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(meta, &m) != nil {
		return false
	}
	_, ok := m["enrichment"]
	return ok
}

// mergeMeta returns meta with key set to v (meta is not modified).
func mergeMeta(meta []byte, key string, v any) []byte {
	m := map[string]any{}
	_ = json.Unmarshal(meta, &m)
	if m == nil {
		m = map[string]any{}
	}
	m[key] = v
	out, _ := json.Marshal(m)
	return out
}

func linkedInSlug(u string) string {
	p, err := url.Parse(u)
	if err != nil || !strings.Contains(p.Host, "linkedin.com") {
		return ""
	}
	parts := strings.Split(strings.Trim(p.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "in" {
		return parts[1]
	}
	return ""
}

func txtIf(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// icpTargetText is the workspace's ICP description (brand profile), "" if none.
func (s *Service) icpTargetText(ctx context.Context, ws string) string {
	bp, err := s.q.GetBrandProfile(ctx, ws)
	if err != nil {
		return ""
	}
	return strings.Join([]string{bp.TargetAudience, string(bp.CustomerSegments), bp.Overview}, " ")
}
