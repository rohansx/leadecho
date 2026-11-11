package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

type fakeLeadStore struct {
	mention   database.Mention
	lead      *database.Lead
	replies   []database.Reply
	brand     database.BrandProfile
	settings  map[string][]byte
	gotWS     string
	gotLeadWS string
}

func (f *fakeLeadStore) GetMention(_ context.Context, a database.GetMentionParams) (database.Mention, error) {
	f.gotWS = a.WorkspaceID
	if a.ID != f.mention.ID || a.WorkspaceID != f.mention.WorkspaceID {
		return database.Mention{}, pgx.ErrNoRows
	}
	return f.mention, nil
}
func (f *fakeLeadStore) GetLeadByMention(_ context.Context, a database.GetLeadByMentionParams) (database.Lead, error) {
	f.gotLeadWS = a.WorkspaceID
	if f.lead == nil {
		return database.Lead{}, pgx.ErrNoRows
	}
	return *f.lead, nil
}
func (f *fakeLeadStore) ListRepliesByMention(context.Context, database.ListRepliesByMentionParams) ([]database.Reply, error) {
	return f.replies, nil
}
func (f *fakeLeadStore) GetBrandProfile(context.Context, string) (database.BrandProfile, error) {
	if f.brand.Product == "" {
		return database.BrandProfile{}, pgx.ErrNoRows
	}
	return f.brand, nil
}
func (f *fakeLeadStore) GetWorkspaceSettingsKey(_ context.Context, _, key string) ([]byte, error) {
	return f.settings[key], nil
}
func (f *fakeLeadStore) SetWorkspaceSettingsKey(_ context.Context, _, key string, v []byte) error {
	if f.settings == nil {
		f.settings = map[string][]byte{}
	}
	f.settings[key] = v
	return nil
}

const (
	leadWS      = "11111111-1111-1111-1111-111111111111"
	leadMention = "22222222-2222-2222-2222-222222222222"
)

func newLeadHandler(s *fakeLeadStore) *ChatHandler {
	return &ChatHandler{leads: s, logger: zerolog.Nop()}
}

func TestLeadContextHiringSignal(t *testing.T) {
	enrich, _ := json.Marshal(map[string]any{"enrichment": map[string]any{
		"full_name": "Marty Frenzel", "title": "VP of Engineering", "company": "Vista Higher Learning",
		"company_domain": "vistahigherlearning.com", "employees": "210", "email_status": "verified",
	}})
	customers, _ := json.Marshal("Northwind Payments: 180-person fintech")
	s := &fakeLeadStore{
		mention: database.Mention{
			ID: leadMention, WorkspaceID: leadWS, Platform: "linkedin", Url: "https://www.linkedin.com/jobs/view/1",
			Title:            pgtype.Text{String: "Vista Higher Learning is hiring: Sr Full stack Software Engineer", Valid: true},
			Content:          "Vista Higher Learning posted a job this week.",
			PlatformMetadata: []byte(`{"signal":"hiring","company":"Vista Higher Learning","job_title":"Sr Full stack Software Engineer","confirmed":true}`),
			ScoringMetadata:  []byte(`{"reasoning":"Company posted a matching job this week.","author_role":"buyer"}`),
			RelevanceScore:   pgtype.Float4{Float32: 7.5, Valid: true},
		},
		lead:     &database.Lead{Metadata: enrich, ContactEmail: pgtype.Text{String: "m@vhl.example", Valid: true}, Stage: database.LeadStageProspect},
		replies:  []database.Reply{{Content: "Subject: Your Sr Full Stack opening\n\nHi Marty,", Status: database.ReplyStatusDraft}},
		brand:    database.BrandProfile{Product: "Utkrusht", Overview: "Technical skill assessments.", TargetAudience: "CTOs at companies under 5,000 employees", CustomerSegments: []byte(`[{"name":"Startup CTOs","why_they_buy":"no time for interview loops"}]`)},
		settings: map[string][]byte{customersSettingsKey: customers},
	}
	got, title, err := newLeadHandler(s).leadContext(context.Background(), leadWS, leadMention)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"hiring signal", "Vista Higher Learning", "Sr Full stack Software Engineer", "Marty Frenzel", "VP of Engineering",
		"vistahigherlearning.com", "210", "yes (verified)", "Hi Marty", "Utkrusht", "CTOs at companies under 5,000 employees",
		"Startup CTOs: no time for interview loops", "Northwind Payments", "web_search", "company_open_jobs",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lead context is missing %q", want)
		}
	}
	if strings.Contains(got, "m@vhl.example") {
		t.Error("the contact's email address must not be put in the prompt, only whether one is on file")
	}
	if title != "Lead: Vista Higher Learning is hiring: Sr Full stack Software Engineer" {
		t.Errorf("title = %q", title)
	}
	if s.gotWS != leadWS || s.gotLeadWS != leadWS {
		t.Errorf("lookups must be scoped to the caller's workspace: %q %q", s.gotWS, s.gotLeadWS)
	}
}

func TestLeadContextPostWithoutLeadOrCustomers(t *testing.T) {
	s := &fakeLeadStore{mention: database.Mention{
		ID: leadMention, WorkspaceID: leadWS, Platform: "hackernews", Url: "https://news.ycombinator.com/item?id=1",
		Content:        "I'll have to create a hiring process to hire a couple of programmers.",
		AuthorUsername: pgtype.Text{String: "Frazzlebilt", Valid: true},
	}}
	got, title, err := newLeadHandler(s).leadContext(context.Background(), leadWS, leadMention)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Frazzlebilt") || !strings.Contains(got, "YOUR CUSTOMERS: none recorded") {
		t.Errorf("context = %s", got)
	}
	if strings.Contains(got, "YOUR PRODUCT (the user's own company)") {
		t.Error("no brand profile: the product section should be absent")
	}
	if !strings.HasPrefix(title, "Lead: I'll have to create a hiring process") {
		t.Errorf("title = %q", title)
	}
}

func TestLeadContextOtherWorkspace(t *testing.T) {
	s := &fakeLeadStore{mention: database.Mention{ID: leadMention, WorkspaceID: leadWS, Content: "x"}}
	if _, _, err := newLeadHandler(s).leadContext(context.Background(), "99999999-9999-9999-9999-999999999999", leadMention); err == nil {
		t.Fatal("a mention from another workspace must not be readable")
	}
}
