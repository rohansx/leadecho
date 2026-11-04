package researcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"leadecho/internal/crmsync"
	"leadecho/internal/database"
	"leadecho/internal/enrich"
	"leadecho/internal/treg"
)

type recCRM struct {
	mu  sync.Mutex
	evs []crmsync.Event
}

func (r *recCRM) Emit(_ string, ev crmsync.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evs = append(r.evs, ev)
}

// DB-backed: a qualified LinkedIn lead goes through the fake-treg pipeline and
// the results land on the lead, the person, a LinkedIn identity and the ICP
// score; a second run pays nothing; the daily cap stops a new lead.
func TestTregEnrichmentDB(t *testing.T) {
	dsn := os.Getenv("LEADECHO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEADECHO_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := database.New(pool)
	var ws string
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (clerk_org_id, name, slug) VALUES ('en-'||gen_random_uuid(), 'en', 'en-'||gen_random_uuid()) RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM workspaces WHERE id = $1`, ws)
	if _, err := pool.Exec(ctx, `INSERT INTO brand_profiles (workspace_id, target_audience) VALUES ($1, 'B2B software teams')`, ws); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	answers := map[string]string{
		enrich.ToolPerson:      `{"output":{"full_name":"Jane Doe","title":"VP Engineering","company":"Acme","company_domain":"acme.io","linkedin_url":"https://www.linkedin.com/in/janedoe"}}`,
		enrich.ToolCompany:     `{"output":{"name":"Acme Inc","domain":"acme.io","industry":"Computer Software","employees":"51-200"}}`,
		enrich.ToolEmailFind:   `{"output":{"email":"jane@acme.io"}}`,
		enrich.ToolEmailVerify: `{"status":"verified"}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(answers[strings.TrimPrefix(r.URL.Path, "/call/")]))
	}))
	defer srv.Close()
	tc := treg.New(treg.Config{Token: "t", BaseURL: srv.URL}, nil, zerolog.Nop())

	s := NewService(q, zerolog.Nop())
	s.SetEnricher(enrich.New(tc, q, 1, 0, zerolog.Nop()))
	crm := &recCRM{}
	s.crm = crm

	newLead := func(handle string) database.Lead {
		l, err := q.CreateLead(ctx, database.CreateLeadParams{
			WorkspaceID: ws, Stage: database.LeadStageProspect,
			Username:   pgtype.Text{String: handle, Valid: true},
			Platform:   database.NullPlatformType{PlatformType: database.PlatformTypeLinkedin, Valid: true},
			ProfileUrl: pgtype.Text{String: "https://www.linkedin.com/in/" + handle, Valid: true},
			Tags:       []string{}, Metadata: []byte(`{}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		return l
	}

	lead := newLead("janedoe")
	if err := s.EnrichLead(ctx, lead.ID, ws); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 4 {
		t.Fatalf("treg calls = %d, want 4", calls.Load())
	}
	got, _ := q.GetLead(ctx, database.GetLeadParams{ID: lead.ID, WorkspaceID: ws})
	if got.ContactName.String != "Jane Doe" || got.ContactEmail.String != "jane@acme.io" || got.Company.String != "Acme Inc" {
		t.Fatalf("lead contact = %q %q %q", got.ContactName.String, got.ContactEmail.String, got.Company.String)
	}
	var meta struct {
		Enrichment enrich.Result `json:"enrichment"`
	}
	_ = json.Unmarshal(got.Metadata, &meta)
	if meta.Enrichment.CompanySize != 51 || meta.Enrichment.EmailStatus != "verified" {
		t.Fatalf("lead metadata = %s", got.Metadata)
	}
	person, err := q.GetPersonByLead(ctx, database.GetPersonByLeadParams{LeadID: lead.ID, WorkspaceID: ws})
	if err != nil {
		t.Fatal(err)
	}
	if person.Company.String != "Acme Inc" || !strings.Contains(string(person.Metadata), `"enrichment"`) || !person.IcpFitScore.Valid {
		t.Fatalf("person = %+v meta=%s", person, person.Metadata)
	}
	ids, _ := q.ListPersonIdentities(ctx, database.ListPersonIdentitiesParams{PersonID: person.ID, WorkspaceID: ws})
	found := false
	for _, id := range ids {
		found = found || (id.Platform == "linkedin" && id.Handle == "janedoe" && id.Confidence >= 0.8 && strings.Contains(string(id.Metadata), "VP Engineering"))
	}
	if !found {
		t.Fatalf("linkedin identity not enriched: %+v", ids)
	}
	crm.mu.Lock()
	if len(crm.evs) != 1 || crm.evs[0].Type != crmsync.EventLeadEnriched {
		t.Fatalf("crm events = %+v", crm.evs)
	}
	crm.mu.Unlock()

	// Re-running pays nothing.
	if err := s.EnrichLead(ctx, lead.ID, ws); err != nil {
		t.Fatal(err)
	}
	// Daily cap (1) reached: a new lead still gets Person360 but no treg calls.
	other := newLead("someoneelse")
	if err := s.EnrichLead(ctx, other.ID, ws); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 4 {
		t.Fatalf("treg calls after rerun + capped lead = %d, want 4", calls.Load())
	}
	if p, err := q.GetPersonByLead(ctx, database.GetPersonByLeadParams{LeadID: other.ID, WorkspaceID: ws}); err != nil || !p.IcpFitScore.Valid {
		t.Fatalf("capped lead person360: %v %+v", err, p)
	}
}
