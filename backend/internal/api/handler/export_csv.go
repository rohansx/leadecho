package handler

// sources: CSV export of mentions and leads.

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

const (
	exportPageSize = 500
	// exportMaxRows caps one export so a huge workspace can't pin a DB
	// connection indefinitely. ponytail: raise or add async exports if needed.
	exportMaxRows = 50000
)

// exportQuerier is the generated Querier plus the hand-written composed list.
type exportQuerier interface {
	database.Querier
	ListMentionsComposed(ctx context.Context, p database.ListMentionsComposedParams) ([]database.Mention, error)
}

// ExportHandler streams workspace-scoped CSV exports.
type ExportHandler struct{ q exportQuerier }

func NewExportHandler(q exportQuerier) *ExportHandler { return &ExportHandler{q: q} }

// csvSafe neutralises spreadsheet formula injection (OWASP): cells starting
// with = + - @ (or tab / CR) are prefixed with a single quote.
// Leading whitespace is skipped first: spreadsheets evaluate " \n=1+1" too.
func csvSafe(s string) string {
	t := strings.TrimLeft(s, " \n")
	if s != "" && (strings.ContainsRune("=+-@\t\r", rune(s[0])) || (t != "" && strings.ContainsRune("=+-@\t\r", rune(t[0])))) {
		return "'" + s
	}
	return s
}

func writeSafeRow(cw *csv.Writer, cells []string) error {
	for i, c := range cells {
		cells[i] = csvSafe(c)
	}
	return cw.Write(cells)
}

func startCSV(w http.ResponseWriter, name string) *csv.Writer {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.csv"`, name, time.Now().UTC().Format("2006-01-02")))
	return csv.NewWriter(w)
}

// mentionFilters parses and validates the mention list filters (shared by
// List and the CSV export). Returns a non-empty message on invalid input.
func mentionFilters(r *http.Request, workspaceID string) (database.ListMentionsComposedParams, string) {
	q := r.URL.Query()
	// All filters compose (ANDed) in a single query rather than being mutually
	// exclusive, and total reflects the full match count, not the page size.
	p := database.ListMentionsComposedParams{
		WorkspaceID:    workspaceID,
		Tier:           q.Get("tier"),
		Queue:          q.Get("queue"),
		ActionKind:     resolveActionKind(q.Get("queue"), q.Get("action_kind"), q.Get("escalation_kind")),
		EscalationKind: q.Get("escalation_kind"),
		Status:         q.Get("status"),
		Platform:       q.Get("platform"),
		Intent:         q.Get("intent"),
		Search:         q.Get("search"),
		Tag:            q.Get("tag"),       // scoring
		Sentiment:      q.Get("sentiment"), // scoring
	}
	if msg := validateTagFilters(p); msg != "" { // scoring
		return p, msg
	}
	// Reject unknown enum filter values up front (400) rather than letting them
	// fail an enum cast deep in the query (500). platform_type is shared with
	// keywords (validKeywordPlatforms).
	switch {
	case p.Status != "" && !validMentionStatuses[p.Status]:
		return p, "invalid status"
	case p.Platform != "" && !validKeywordPlatforms[p.Platform]:
		return p, "invalid platform"
	case p.Intent != "" && !validMentionIntents[p.Intent]:
		return p, "invalid intent"
	case p.Queue != "" && !validMentionQueues[p.Queue]:
		return p, "invalid queue"
	case p.EscalationKind != "" && p.Queue != "escalations":
		return p, "escalation_kind requires queue=escalations"
	case p.EscalationKind != "" && !validEscalationKinds[p.EscalationKind]:
		return p, "invalid escalation_kind"
	case p.ActionKind != "" && p.Queue != "action_required":
		return p, "action_kind requires queue=action_required"
	case p.ActionKind != "" && !validActionKinds[p.ActionKind]:
		return p, "invalid action_kind"
	}
	return p, ""
}

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func fmtF32(v float32, ok bool) string {
	if !ok {
		return ""
	}
	return strconv.FormatFloat(float64(v), 'f', 3, 32)
}

// Mentions handles GET /mentions/export.csv (same filters as GET /mentions).
func (h *ExportHandler) Mentions(w http.ResponseWriter, r *http.Request) {
	// Up to 50k rows outlasts the server's 30s WriteTimeout; don't cut the file mid-stream.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Minute))
	ctx := r.Context()
	params, errMsg := mentionFilters(r, middleware.WorkspaceID(ctx))
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	params.Lim = exportPageSize

	// Fetch the first page before committing to a 200 so a DB error is a 500.
	page, err := h.q.ListMentionsComposed(ctx, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to export mentions")
		return
	}
	cw := startCSV(w, "mentions")
	_ = cw.Write([]string{"id", "created_at", "platform_created_at", "platform", "status", "intent", "relevance_score", "conversion_probability", "url", "title", "author_username", "author_profile_url", "keyword_matches", "content"})
	for n := 0; len(page) > 0 && n < exportMaxRows; {
		for _, m := range page {
			posted := ""
			if m.PlatformCreatedAt.Valid {
				posted = fmtTime(m.PlatformCreatedAt.Time)
			}
			if err := writeSafeRow(cw, []string{
				m.ID, fmtTime(m.CreatedAt), posted, string(m.Platform), string(m.Status), string(m.Intent.IntentType),
				fmtF32(m.RelevanceScore.Float32, m.RelevanceScore.Valid), fmtF32(m.ConversionProbability.Float32, m.ConversionProbability.Valid),
				m.Url, m.Title.String, m.AuthorUsername.String, m.AuthorProfileUrl.String, strings.Join(m.KeywordMatches, ";"), m.Content,
			}); err != nil {
				return // client went away
			}
		}
		n += len(page)
		if len(page) < exportPageSize {
			break
		}
		cw.Flush()
		params.Off += exportPageSize
		if page, err = h.q.ListMentionsComposed(ctx, params); err != nil {
			break // headers already sent; truncated export is the best we can do
		}
	}
	cw.Flush()
}

// Leads handles GET /leads/export.csv (same ?stage= filter as GET /leads).
func (h *ExportHandler) Leads(w http.ResponseWriter, r *http.Request) {
	// Up to 50k rows outlasts the server's 30s WriteTimeout; don't cut the file mid-stream.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Minute))
	ctx := r.Context()
	ws := middleware.WorkspaceID(ctx)
	stage := r.URL.Query().Get("stage")
	if stage != "" && !validLeadStages[stage] {
		writeError(w, http.StatusBadRequest, "invalid stage")
		return
	}
	fetch := func(off int32) ([]database.Lead, error) {
		if stage != "" {
			return h.q.ListLeadsByStage(ctx, database.ListLeadsByStageParams{WorkspaceID: ws, Stage: database.LeadStage(stage), Lim: exportPageSize, Off: off})
		}
		return h.q.ListLeads(ctx, database.ListLeadsParams{WorkspaceID: ws, Lim: exportPageSize, Off: off})
	}
	page, err := fetch(0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to export leads")
		return
	}
	cw := startCSV(w, "leads")
	_ = cw.Write([]string{"id", "created_at", "stage", "contact_name", "contact_email", "company", "username", "platform", "profile_url", "estimated_value", "tags", "notes"})
	for off := int32(0); len(page) > 0 && off < exportMaxRows; {
		for _, l := range page {
			value := ""
			if l.EstimatedValue.Valid {
				value = strconv.Itoa(int(l.EstimatedValue.Int32))
			}
			if err := writeSafeRow(cw, []string{
				l.ID, fmtTime(l.CreatedAt), string(l.Stage), l.ContactName.String, l.ContactEmail.String, l.Company.String,
				l.Username.String, string(l.Platform.PlatformType), l.ProfileUrl.String, value, strings.Join(l.Tags, ";"), l.Notes.String,
			}); err != nil {
				return
			}
		}
		if len(page) < exportPageSize {
			break
		}
		cw.Flush()
		off += exportPageSize
		if page, err = fetch(off); err != nil {
			break
		}
	}
	cw.Flush()
}
