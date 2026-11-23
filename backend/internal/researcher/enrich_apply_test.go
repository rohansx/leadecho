package researcher

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/database"
	"leadecho/internal/enrich"
)

func txt(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func TestLeadUpdateFromEnrichment(t *testing.T) {
	full := &enrich.Result{FullName: "Jane Doe", Company: "Acme", Email: "jane@acme.io", EmailStatus: "verified"}
	cases := []struct {
		name                 string
		lead                 database.Lead
		res                  *enrich.Result
		wantName, wantEmail  string
		wantCompany, wantErr string
	}{
		{"fills empty contact", database.Lead{Stage: database.LeadStageProspect}, full, "Jane Doe", "jane@acme.io", "Acme", ""},
		{"never overwrites what a human typed", database.Lead{ContactName: txt("J. D."), ContactEmail: txt("me@x.com"), Company: txt("X")}, full, "", "", "", ""},
		{"invalid email is not stored as the contact", database.Lead{}, &enrich.Result{Email: "bad@acme.io", EmailStatus: "invalid"}, "", "", "", ""},
		{"unverified (catchall) email is kept", database.Lead{}, &enrich.Result{Email: "c@acme.io", EmailStatus: "catchall"}, "", "c@acme.io", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.lead.ID, c.lead.WorkspaceID = "l1", "w1"
			p := leadUpdateFromEnrichment(c.lead, c.res)
			if p.ContactName.String != c.wantName || p.ContactEmail.String != c.wantEmail || p.Company.String != c.wantCompany {
				t.Errorf("got name=%q email=%q company=%q", p.ContactName.String, p.ContactEmail.String, p.Company.String)
			}
			if p.ContactName.Valid != (c.wantName != "") || p.ContactEmail.Valid != (c.wantEmail != "") {
				t.Errorf("validity flags wrong: %+v", p)
			}
			if p.Stage != c.lead.Stage || p.ID != "l1" || p.WorkspaceID != "w1" {
				t.Errorf("stage/id must be carried over: %+v", p)
			}
		})
	}
}

func TestEnrichSubject(t *testing.T) {
	lead := database.Lead{ContactName: txt("Jane Doe"), Company: txt("Acme"), ContactEmail: txt("")}
	s := enrichSubject(lead, "linkedin", "janedoe", "https://www.linkedin.com/in/janedoe")
	want := enrich.Subject{Name: "Jane Doe", Handle: "janedoe", Platform: "linkedin", ProfileURL: "https://www.linkedin.com/in/janedoe", Company: "Acme"}
	if s != want {
		t.Fatalf("got %+v", s)
	}
}

func TestNeedsEnrichment(t *testing.T) {
	bare := `{"enrichment":{"steps":{"treg.people.enrich":"skipped"}}}`
	found := `{"enrichment":{"full_name":"Jane","steps":{"treg.people.enrich":"hit"}}}`
	handleOnly := enrich.Subject{Platform: "reddit", Handle: "jane"}
	withKey := enrich.Subject{Platform: "reddit", Handle: "jane", ProfileURL: "https://www.linkedin.com/in/jane"}
	cases := []struct {
		name, meta string
		sub        enrich.Subject
		want       bool
	}{
		{"never enriched", `{}`, handleOnly, true},
		{"unreadable metadata", `not json`, handleOnly, true},
		{"enriched as a bare handle, still no key", bare, handleOnly, false},
		{"enriched as a bare handle, now a LinkedIn key", bare, withKey, true},
		{"enriched as a bare handle, now a published email", bare, enrich.Subject{Email: "j@jane.dev"}, true},
		{"already found: never pay twice", found, withKey, false},
	}
	for _, c := range cases {
		if got := needsEnrichment([]byte(c.meta), c.sub); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMergeMeta(t *testing.T) {
	out := mergeMeta([]byte(`{"public_repos":3}`), "enrichment", map[string]string{"title": "CTO"})
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["public_repos"] != float64(3) || m["enrichment"].(map[string]any)["title"] != "CTO" {
		t.Fatalf("got %s", out)
	}
	if string(mergeMeta(nil, "k", 1)) != `{"k":1}` {
		t.Fatalf("nil meta: %s", mergeMeta(nil, "k", 1))
	}
}
