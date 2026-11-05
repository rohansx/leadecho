package crmsync

// enrich: treg enrichment (leads.metadata.enrichment, written by the
// researcher) folded into the payload and the HubSpot/Attio field maps.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// EventLeadEnriched fires once a lead's contact/company data was enriched.
const EventLeadEnriched = "lead.enriched"

type enrichmentMeta struct {
	Enrichment struct {
		Title         string `json:"title"`
		Industry      string `json:"industry"`
		CompanyDomain string `json:"company_domain"`
		CompanySize   int    `json:"company_size"`
		LinkedInURL   string `json:"linkedin_url"`
		EmailStatus   string `json:"email_status"`
	} `json:"enrichment"`
}

func withEnrichment(p LeadPayload, meta []byte) LeadPayload {
	var m enrichmentMeta
	if len(meta) == 0 || json.Unmarshal(meta, &m) != nil {
		return p
	}
	e := m.Enrichment
	p.JobTitle, p.Industry, p.CompanyDomain = e.Title, e.Industry, e.CompanyDomain
	p.CompanySize, p.LinkedInURL, p.EmailStatus = e.CompanySize, e.LinkedInURL, e.EmailStatus
	return p
}

// hubspotEnriched adds enrichment-backed contact properties. Only free-text
// default properties: numemployees is an enumeration on contacts, so size
// goes out on the webhook and the Attio note instead.
func hubspotEnriched(props map[string]string, p LeadPayload, withContact bool) {
	set := func(k, v string) {
		if v != "" {
			props[k] = v
		}
	}
	set("jobtitle", p.JobTitle)
	set("industry", p.Industry)
	if withContact {
		set("email", p.ContactEmail)
		set("company", p.Company)
	}
}

// attioEnriched is the people-record values patch for an enriched lead.
func attioEnriched(p LeadPayload) map[string]any {
	v := map[string]any{}
	if p.JobTitle != "" {
		v["job_title"] = p.JobTitle
	}
	if p.ContactEmail != "" {
		v["email_addresses"] = []string{p.ContactEmail}
	}
	return v
}

func enrichmentNote(b *strings.Builder, p LeadPayload) {
	line := func(label, v string) {
		if v != "" {
			fmt.Fprintf(b, "\n%s: %s", label, v)
		}
	}
	line("Title", p.JobTitle)
	line("Company", p.Company)
	line("Industry", p.Industry)
	if p.CompanySize > 0 {
		fmt.Fprintf(b, "\nCompany size: %d+", p.CompanySize)
	}
	if p.ContactEmail != "" {
		line("Email", strings.TrimSpace(p.ContactEmail+" "+paren(p.EmailStatus)))
	}
	line("LinkedIn", p.LinkedInURL)
}

func paren(s string) string {
	if s == "" {
		return ""
	}
	return "(" + s + ")"
}
