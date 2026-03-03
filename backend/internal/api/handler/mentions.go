package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

type MentionHandler struct {
	q *database.Queries
}

func NewMentionHandler(q *database.Queries) *MentionHandler {
	return &MentionHandler{q: q}
}

// MentionResponse is the JSON-friendly representation of a mention.
type MentionResponse struct {
	ID                    string          `json:"id"`
	WorkspaceID           string          `json:"workspace_id"`
	KeywordID             *string         `json:"keyword_id"`
	Platform              string          `json:"platform"`
	PlatformID            string          `json:"platform_id"`
	URL                   string          `json:"url"`
	Title                 *string         `json:"title"`
	Content               string          `json:"content"`
	AuthorUsername         *string         `json:"author_username"`
	AuthorProfileURL      *string         `json:"author_profile_url"`
	AuthorKarma           *int32          `json:"author_karma"`
	AuthorAccountAgeDays  *int32          `json:"author_account_age_days"`
	RelevanceScore        *float32        `json:"relevance_score"`
	Intent                *string         `json:"intent"`
	ConversionProbability *float32        `json:"conversion_probability"`
	Status                string          `json:"status"`
	AssignedTo            *string         `json:"assigned_to"`
	PlatformMetadata      json.RawMessage `json:"platform_metadata"`
	EngagementMetrics     json.RawMessage `json:"engagement_metrics"`
	ScoringMetadata       json.RawMessage `json:"scoring_metadata"`
	KeywordMatches        []string        `json:"keyword_matches"`
	PlatformCreatedAt     *time.Time      `json:"platform_created_at"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

func mentionToResponse(m database.Mention) MentionResponse {
	r := MentionResponse{
		ID:                m.ID,
		WorkspaceID:       m.WorkspaceID,
		Platform:          string(m.Platform),
		PlatformID:        m.PlatformID,
		URL:               m.Url,
		Content:           m.Content,
		Status:            string(m.Status),
		PlatformMetadata:  json.RawMessage(m.PlatformMetadata),
		EngagementMetrics: json.RawMessage(m.EngagementMetrics),
		ScoringMetadata:   json.RawMessage(m.ScoringMetadata),
		KeywordMatches:    m.KeywordMatches,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
	if m.KeywordID.Valid {
		s := uuidToString(m.KeywordID)
		r.KeywordID = &s
	}
	if m.Title.Valid {
		r.Title = &m.Title.String
	}
	if m.AuthorUsername.Valid {
		r.AuthorUsername = &m.AuthorUsername.String
	}
	if m.AuthorProfileUrl.Valid {
		r.AuthorProfileURL = &m.AuthorProfileUrl.String
	}
	if m.AuthorKarma.Valid {
		r.AuthorKarma = &m.AuthorKarma.Int32
	}
	if m.AuthorAccountAgeDays.Valid {
		r.AuthorAccountAgeDays = &m.AuthorAccountAgeDays.Int32
	}
	if m.RelevanceScore.Valid {
		r.RelevanceScore = &m.RelevanceScore.Float32
	}
	if m.Intent.Valid {
		s := string(m.Intent.IntentType)
		r.Intent = &s
	}
	if m.ConversionProbability.Valid {
		r.ConversionProbability = &m.ConversionProbability.Float32
	}
	if m.AssignedTo.Valid {
		s := uuidToString(m.AssignedTo)
		r.AssignedTo = &s
	}
	if m.PlatformCreatedAt.Valid {
		r.PlatformCreatedAt = &m.PlatformCreatedAt.Time
	}
	return r
}

func uuidToString(u pgtype.UUID) string {
	return uuidBytesToString(u.Bytes)
}

func uuidBytesToString(b [16]byte) string {
	const hex = "0123456789abcdef"
	var buf [36]byte
	pos := 0
	for i, v := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			buf[pos] = '-'
			pos++
		}
		buf[pos] = hex[v>>4]
		buf[pos+1] = hex[v&0x0f]
		pos += 2
	}
	return string(buf[:])
}

func (h *MentionHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceID := middleware.WorkspaceID(ctx)

	limit := int32(20)
	offset := int32(0)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n)
		}
	}

	var mentions []database.Mention
	var err error

	tier := r.URL.Query().Get("tier")
	status := r.URL.Query().Get("status")
	platform := r.URL.Query().Get("platform")
	intent := r.URL.Query().Get("intent")
	search := r.URL.Query().Get("search")

	switch {
	case tier == "leads_ready":
		mentions, err = h.q.ListMentionsLeadsReady(ctx, database.ListMentionsLeadsReadyParams{
			WorkspaceID: workspaceID,
			Lim:         limit,
			Off:         offset,
		})
	case tier == "worth_watching":
		mentions, err = h.q.ListMentionsWorthWatching(ctx, database.ListMentionsWorthWatchingParams{
			WorkspaceID: workspaceID,
			Lim:         limit,
			Off:         offset,
		})
	case tier == "filtered":
		mentions, err = h.q.ListMentionsFiltered(ctx, database.ListMentionsFilteredParams{
			WorkspaceID: workspaceID,
			Lim:         limit,
			Off:         offset,
		})
	case search != "":
		mentions, err = h.q.SearchMentions(ctx, database.SearchMentionsParams{
			WorkspaceID: workspaceID,
			Query:       search,
			Lim:         limit,
			Off:         offset,
		})
	case status != "":
		mentions, err = h.q.ListMentionsByStatus(ctx, database.ListMentionsByStatusParams{
			WorkspaceID: workspaceID,
			Status:      database.MentionStatus(status),
			Lim:         limit,
			Off:         offset,
		})
	case platform != "":
		mentions, err = h.q.ListMentionsByPlatform(ctx, database.ListMentionsByPlatformParams{
			WorkspaceID: workspaceID,
			Platform:    platform,
			Lim:         limit,
			Off:         offset,
		})
	case intent != "":
		mentions, err = h.q.ListMentionsByIntent(ctx, database.ListMentionsByIntentParams{
			WorkspaceID: workspaceID,
			Intent:      database.NullIntentType{IntentType: database.IntentType(intent), Valid: true},
			Lim:         limit,
			Off:         offset,
		})
	default:
		mentions, err = h.q.ListMentions(ctx, database.ListMentionsParams{
			WorkspaceID: workspaceID,
			Lim:         limit,
			Off:         offset,
		})
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list mentions")
		return
	}

	resp := make([]MentionResponse, len(mentions))
	for i, m := range mentions {
		resp[i] = mentionToResponse(m)
	}

	writeJSON(w, http.StatusOK, listResponse{
		Data:   resp,
		Total:  len(resp),
		Limit:  limit,
		Offset: offset,
	})
}

func (h *MentionHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	workspaceID := middleware.WorkspaceID(ctx)

	m, err := h.q.GetMention(ctx, database.GetMentionParams{
		ID:          id,
		WorkspaceID: workspaceID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "mention not found")
		return
	}

	writeJSON(w, http.StatusOK, mentionToResponse(m))
}

func (h *MentionHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	workspaceID := middleware.WorkspaceID(ctx)

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	m, err := h.q.UpdateMentionStatus(ctx, database.UpdateMentionStatusParams{
		Status:      database.MentionStatus(body.Status),
		ID:          id,
		WorkspaceID: workspaceID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update mention")
		return
	}

	writeJSON(w, http.StatusOK, mentionToResponse(m))
}

func (h *MentionHandler) Counts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceID := middleware.WorkspaceID(ctx)

	counts, err := h.q.CountMentionsByStatus(ctx, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count mentions")
		return
	}

	type countItem struct {
		Status string `json:"status"`
		Count  int32  `json:"count"`
	}
	resp := make([]countItem, len(counts))
	for i, c := range counts {
		resp[i] = countItem{Status: string(c.Status), Count: c.Count}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *MentionHandler) TierCounts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceID := middleware.WorkspaceID(ctx)

	counts, err := h.q.CountMentionsByTier(ctx, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count mentions by tier")
		return
	}

	type tierItem struct {
		Tier  string `json:"tier"`
		Count int32  `json:"count"`
	}
	resp := make([]tierItem, len(counts))
	for i, c := range counts {
		resp[i] = tierItem{Tier: c.Tier, Count: c.Count}
	}

	writeJSON(w, http.StatusOK, resp)
}
