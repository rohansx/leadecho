package tools

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/crmsync"
	"leadecho/internal/database"
	"leadecho/internal/reply"
)

const (
	ws      = "11111111-1111-1111-1111-111111111111"
	otherWS = "22222222-2222-2222-2222-222222222222"
	leadID  = "33333333-3333-3333-3333-333333333333"
	mentID  = "44444444-4444-4444-4444-444444444444"
	profID  = "55555555-5555-5555-5555-555555555555"
	replyID = "66666666-6666-6666-6666-666666666666"
	kwID    = "77777777-7777-7777-7777-777777777777"
	missing = "99999999-9999-9999-9999-999999999999"
)

// fakeQ implements the subset of database.Querier the tools use; any other
// method panics via the nil embedded interface. Rows are keyed by workspace so
// cross-tenant reads miss, as the real WHERE workspace_id clauses do.
type fakeQ struct {
	database.Querier
	mu sync.Mutex

	lastCall    string
	lastLimit   int32
	lastOffset  int32
	lastQuery   string
	createdKW   []database.CreateKeywordParams
	deletedKW   []string
	intent      *database.ToolsUpdateListenerIntentParams
	approved    *database.ApproveReplyParams
	edited      *database.UpdateReplyContentParams
	stage       *database.UpdateLeadStageParams
	leadEvents  int
	replyStatus database.ReplyStatus
}

func (f *fakeQ) rec(name string, lim, off int32) []database.Mention {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastCall, f.lastLimit, f.lastOffset = name, lim, off
	return []database.Mention{sampleMention()}
}

func sampleMention() database.Mention {
	return database.Mention{
		ID: mentID, WorkspaceID: ws, Platform: "reddit", Url: "https://reddit.com/x",
		Content:        "looking for a tool like this",
		Title:          pgtype.Text{String: "Need help", Valid: true},
		AuthorUsername: pgtype.Text{String: "alice", Valid: true},
		RelevanceScore: pgtype.Float4{Float32: 8.5, Valid: true},
		Intent:         database.NullIntentType{IntentType: "buy_signal", Valid: true},
		Status:         "new",
		CreatedAt:      time.Unix(0, 0).UTC(),
	}
}

func (f *fakeQ) SearchMentions(_ context.Context, a database.SearchMentionsParams) ([]database.Mention, error) {
	f.lastQuery = a.Query
	return f.rec("SearchMentions", a.Lim, a.Off), nil
}
func (f *fakeQ) ListMentions(_ context.Context, a database.ListMentionsParams) ([]database.Mention, error) {
	return f.rec("ListMentions", a.Lim, a.Off), nil
}
func (f *fakeQ) ListMentionsByStatus(_ context.Context, a database.ListMentionsByStatusParams) ([]database.Mention, error) {
	return f.rec("ListMentionsByStatus", a.Lim, a.Off), nil
}
func (f *fakeQ) ListMentionsByIntent(_ context.Context, a database.ListMentionsByIntentParams) ([]database.Mention, error) {
	return f.rec("ListMentionsByIntent", a.Lim, a.Off), nil
}
func (f *fakeQ) ListMentionsByPlatform(_ context.Context, a database.ListMentionsByPlatformParams) ([]database.Mention, error) {
	return f.rec("ListMentionsByPlatform", a.Lim, a.Off), nil
}
func (f *fakeQ) ListMentionsLeadsReady(_ context.Context, a database.ListMentionsLeadsReadyParams) ([]database.Mention, error) {
	return f.rec("ListMentionsLeadsReady", a.Lim, a.Off), nil
}
func (f *fakeQ) GetMention(_ context.Context, a database.GetMentionParams) (database.Mention, error) {
	if a.ID != mentID || a.WorkspaceID != ws {
		return database.Mention{}, pgx.ErrNoRows
	}
	return sampleMention(), nil
}

func sampleLead() database.Lead {
	return database.Lead{
		ID: leadID, WorkspaceID: ws, Stage: "prospect", Tags: []string{},
		MentionID: pgtype.UUID{Bytes: [16]byte{0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44}, Valid: true},
		Username:  pgtype.Text{String: "alice", Valid: true},
		Metadata:  []byte(`{}`),
	}
}

func (f *fakeQ) ListLeads(_ context.Context, a database.ListLeadsParams) ([]database.Lead, error) {
	f.lastCall, f.lastLimit, f.lastOffset = "ListLeads", a.Lim, a.Off
	return []database.Lead{sampleLead()}, nil
}
func (f *fakeQ) ListLeadsByStage(_ context.Context, a database.ListLeadsByStageParams) ([]database.Lead, error) {
	f.lastCall, f.lastLimit, f.lastOffset = "ListLeadsByStage:"+string(a.Stage), a.Lim, a.Off
	return []database.Lead{sampleLead()}, nil
}
func (f *fakeQ) GetLead(_ context.Context, a database.GetLeadParams) (database.Lead, error) {
	if a.ID != leadID || a.WorkspaceID != ws {
		return database.Lead{}, pgx.ErrNoRows
	}
	return sampleLead(), nil
}
func (f *fakeQ) ListLeadEvents(context.Context, string) ([]database.LeadEvent, error) {
	return []database.LeadEvent{{ID: "e1", LeadID: leadID, NewStage: "prospect"}}, nil
}
func (f *fakeQ) UpdateLeadStage(_ context.Context, a database.UpdateLeadStageParams) (database.Lead, error) {
	f.stage = &a
	l := sampleLead()
	l.Stage = a.Stage
	return l, nil
}
func (f *fakeQ) CreateLeadEvent(context.Context, database.CreateLeadEventParams) (database.LeadEvent, error) {
	f.leadEvents++
	return database.LeadEvent{}, nil
}

func (f *fakeQ) CreateMonitoringProfile(_ context.Context, a database.CreateMonitoringProfileParams) (database.MonitoringProfile, error) {
	return database.MonitoringProfile{ID: profID, WorkspaceID: a.WorkspaceID, Name: a.Name, Description: a.Description, IsActive: a.IsActive}, nil
}
func (f *fakeQ) GetMonitoringProfile(_ context.Context, a database.GetMonitoringProfileParams) (database.MonitoringProfile, error) {
	if a.ID != profID || a.WorkspaceID != ws {
		return database.MonitoringProfile{}, pgx.ErrNoRows
	}
	return database.MonitoringProfile{ID: profID, WorkspaceID: ws, Name: "Buyers", LooksFor: "old", Skips: "old skips"}, nil
}
func (f *fakeQ) ToolsUpdateListenerIntent(_ context.Context, a database.ToolsUpdateListenerIntentParams) (database.MonitoringProfile, error) {
	f.intent = &a
	return database.MonitoringProfile{ID: a.ID, WorkspaceID: a.WorkspaceID, Name: "Buyers", LooksFor: a.LooksFor, Skips: a.Skips}, nil
}
func (f *fakeQ) CreateKeyword(_ context.Context, a database.CreateKeywordParams) (database.CreateKeywordRow, error) {
	if a.Term == "dupe" {
		return database.CreateKeywordRow{}, &pgconn.PgError{Code: "23505"}
	}
	f.createdKW = append(f.createdKW, a)
	return database.CreateKeywordRow{ID: "kw-new", Term: a.Term, ProfileID: a.ProfileID}, nil
}
func (f *fakeQ) ListKeywords(_ context.Context, wsID string) ([]database.ListKeywordsRow, error) {
	if wsID != ws {
		return nil, nil
	}
	return []database.ListKeywordsRow{
		{ID: kwID, WorkspaceID: ws, ProfileID: profID, Term: "CRM Alternative"},
		{ID: "kw-other-profile", WorkspaceID: ws, ProfileID: "other", Term: "crm alternative"},
	}, nil
}
func (f *fakeQ) DeleteKeyword(_ context.Context, a database.DeleteKeywordParams) error {
	f.deletedKW = append(f.deletedKW, a.ID)
	return nil
}

func (f *fakeQ) GetReply(_ context.Context, a database.GetReplyParams) (database.Reply, error) {
	if a.ID != replyID || a.WorkspaceID != ws {
		return database.Reply{}, pgx.ErrNoRows
	}
	st := f.replyStatus
	if st == "" {
		st = database.ReplyStatusDraft
	}
	return database.Reply{ID: replyID, MentionID: mentID, WorkspaceID: ws, Content: "hi", Status: st}, nil
}
func (f *fakeQ) UpdateReplyContent(_ context.Context, a database.UpdateReplyContentParams) (database.Reply, error) {
	f.edited = &a
	return database.Reply{ID: replyID, MentionID: mentID, WorkspaceID: ws, Content: "hi", EditedContent: a.EditedContent, Status: "draft"}, nil
}
func (f *fakeQ) ApproveReply(_ context.Context, a database.ApproveReplyParams) (database.Reply, error) {
	f.approved = &a
	return database.Reply{ID: replyID, MentionID: mentID, WorkspaceID: ws, Content: "hi", EditedContent: a.EditedContent, Status: "approved"}, nil
}

func (f *fakeQ) ToolsAttributionByPlatform(_ context.Context, a database.ToolsAttributionByPlatformParams) ([]database.ToolsAttributionByPlatformRow, error) {
	f.lastLimit = a.Days
	return []database.ToolsAttributionByPlatformRow{
		{Platform: "reddit", RepliesPosted: 4, Clicks: 30, Signups: 3, RevenueCents: 12900},
		{Platform: "hackernews", RepliesPosted: 1, Clicks: 5, Signups: 0, RevenueCents: 0},
	}, nil
}
func (f *fakeQ) ToolsWorkspaceSummary(_ context.Context, a database.ToolsWorkspaceSummaryParams) (database.ToolsWorkspaceSummaryRow, error) {
	f.lastLimit = a.Days
	return database.ToolsWorkspaceSummaryRow{Mentions: 40, HighIntent: 12, Complaints: 4, NewLeads: 3, RepliesPosted: 5, RevenueCents: 12900}, nil
}
func (f *fakeQ) TopKeywords(context.Context, string) ([]database.TopKeywordsRow, error) {
	return []database.TopKeywordsRow{{Term: "crm", MentionCount: 9}}, nil
}

type fakeDrafter struct{ err error }

func (d fakeDrafter) DraftForMention(_ context.Context, wsID, mentionID string) (*reply.DraftResult, error) {
	if d.err != nil {
		return nil, d.err
	}
	if wsID != ws || mentionID != mentID {
		return nil, pgx.ErrNoRows
	}
	return &reply.DraftResult{ShouldReply: true, Reply: database.Reply{ID: replyID, MentionID: mentID, Content: "drafted", Status: "draft"}, TemplateStyle: "value_first"}, nil
}

type fakeCRM struct{ events []crmsync.Event }

func (c *fakeCRM) Emit(_ string, ev crmsync.Event) { c.events = append(c.events, ev) }

type fakeMutes struct{ got []Mute }

func (m *fakeMutes) Mute(_ context.Context, _ string, mu Mute) error {
	if mu.Value == "boom" {
		return errors.New("db down")
	}
	m.got = append(m.got, mu)
	return nil
}
