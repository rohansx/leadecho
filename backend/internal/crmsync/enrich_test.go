package crmsync

// enrich: treg enrichment stored on leads.metadata.enrichment reaches every sink.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

const enrichedMeta = `{"enrichment":{"title":"VP Engineering","company_domain":"acme.io","industry":"Computer Software","company_size":51,"linkedin_url":"https://www.linkedin.com/in/ada","email_status":"verified"}}`

func TestPayloadCarriesEnrichment(t *testing.T) {
	p := toPayload(lead(enrichedMeta))
	if p.JobTitle != "VP Engineering" || p.Industry != "Computer Software" || p.CompanySize != 51 ||
		p.CompanyDomain != "acme.io" || p.LinkedInURL != "https://www.linkedin.com/in/ada" || p.EmailStatus != "verified" {
		t.Fatalf("payload = %+v", p)
	}
	if empty := toPayload(lead(`{}`)); empty.JobTitle != "" || empty.CompanySize != 0 {
		t.Fatalf("unenriched payload = %+v", empty)
	}
}

func TestHubSpotEnrichedFields(t *testing.T) {
	cases := []struct {
		name, meta string
		ev         Event
		wantMethod string
		wantPath   string
		want       []string
	}{
		{"create carries title and industry", enrichedMeta, Event{Type: EventLeadCreated}, http.MethodPost, "/crm/v3/objects/contacts",
			[]string{`"jobtitle":"VP Engineering"`, `"industry":"Computer Software"`, `"email":"ada@acme.io"`}},
		{"enriched event patches the existing contact", `{"hubspot_contact_id":"901",` + enrichedMeta[1:], Event{Type: EventLeadEnriched}, http.MethodPatch, "/crm/v3/objects/contacts/901",
			[]string{`"jobtitle":"VP Engineering"`, `"email":"ada@acme.io"`, `"company":"Acme"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, calls := vendor(t, map[string]string{"POST /crm/v3/objects/contacts": `{"id":"901"}`})
			d := testDispatcher(&fakeDB{}, srv.URL)
			l := lead(tc.meta)
			l.ContactEmail = pgtype.Text{String: "ada@acme.io", Valid: true}
			l.Company = pgtype.Text{String: "Acme", Valid: true}
			if err := d.syncHubSpot(context.Background(), "ws", "tok", tc.ev, l, toPayload(l)); err != nil {
				t.Fatal(err)
			}
			c := (*calls)[0]
			if c.method != tc.wantMethod || c.path != tc.wantPath {
				t.Fatalf("call = %s %s", c.method, c.path)
			}
			for _, w := range tc.want {
				if !strings.Contains(c.body, w) {
					t.Errorf("body %s missing %s", c.body, w)
				}
			}
		})
	}
}

func TestAttioEnrichedPatchesPerson(t *testing.T) {
	srv, calls := vendor(t, nil)
	d := testDispatcher(&fakeDB{}, srv.URL)
	l := lead(`{"attio_record_id":"rec-1",` + enrichedMeta[1:])
	l.ContactEmail = pgtype.Text{String: "ada@acme.io", Valid: true}
	if err := d.syncAttio(context.Background(), "ws", "tok", Event{Type: EventLeadEnriched}, l, toPayload(l)); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 {
		t.Fatalf("want patch + note, got %+v", *calls)
	}
	patch, note := (*calls)[0], (*calls)[1]
	if patch.method != http.MethodPatch || patch.path != "/v2/objects/people/records/rec-1" ||
		!strings.Contains(patch.body, `"job_title":"VP Engineering"`) || !strings.Contains(patch.body, `"email_addresses":["ada@acme.io"]`) {
		t.Fatalf("patch = %+v", patch)
	}
	if note.path != "/v2/notes" || !strings.Contains(note.body, "VP Engineering") || !strings.Contains(note.body, "Computer Software") {
		t.Fatalf("note = %+v", note)
	}
}
