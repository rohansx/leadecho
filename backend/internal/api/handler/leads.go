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

type LeadHandler struct {
	q *database.Queries
}

func NewLeadHandler(q *database.Queries) *LeadHandler {
	return &LeadHandler{q: q}
}

type LeadResponse struct {
	ID             string          `json:"id"`
	WorkspaceID    string          `json:"workspace_id"`
	MentionID      *string         `json:"mention_id"`
	Stage          string          `json:"stage"`
	ContactName    *string         `json:"contact_name"`
	ContactEmail   *string         `json:"contact_email"`
	Company        *string         `json:"company"`
	Username       *string         `json:"username"`
	Platform       *string         `json:"platform"`
	ProfileURL     *string         `json:"profile_url"`
	EstimatedValue *int32          `json:"estimated_value"`
	Notes          *string         `json:"notes"`
	Tags           []string        `json:"tags"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func leadToResponse(l database.Lead) LeadResponse {
	r := LeadResponse{
		ID:          l.ID,
		WorkspaceID: l.WorkspaceID,
		Stage:       string(l.Stage),
		Tags:        l.Tags,
		Metadata:    json.RawMessage(l.Metadata),
		CreatedAt:   l.CreatedAt,
		UpdatedAt:   l.UpdatedAt,
	}
	if l.MentionID.Valid {
		s := uuidBytesToString(l.MentionID.Bytes)
		r.MentionID = &s
	}
	if l.ContactName.Valid {
		r.ContactName = &l.ContactName.String
	}
	if l.ContactEmail.Valid {
		r.ContactEmail = &l.ContactEmail.String
	}
	if l.Company.Valid {
		r.Company = &l.Company.String
	}
	if l.Username.Valid {
		r.Username = &l.Username.String
	}
	if l.Platform.Valid {
		s := string(l.Platform.PlatformType)
		r.Platform = &s
	}
	if l.ProfileUrl.Valid {
		r.ProfileURL = &l.ProfileUrl.String
	}
	if l.EstimatedValue.Valid {
		r.EstimatedValue = &l.EstimatedValue.Int32
	}
	if l.Notes.Valid {
		r.Notes = &l.Notes.String
	}
	return r
}

func (h *LeadHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceID := middleware.WorkspaceID(ctx)

	limit := int32(50)
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

	var leads []database.Lead
	var err error

	stage := r.URL.Query().Get("stage")
	if stage != "" {
		leads, err = h.q.ListLeadsByStage(ctx, database.ListLeadsByStageParams{
			WorkspaceID: workspaceID,
			Stage:       database.LeadStage(stage),
			Lim:         limit,
			Off:         offset,
		})
	} else {
		leads, err = h.q.ListLeads(ctx, database.ListLeadsParams{
			WorkspaceID: workspaceID,
			Lim:         limit,
			Off:         offset,
		})
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list leads")
		return
	}

	resp := make([]LeadResponse, len(leads))
	for i, l := range leads {
		resp[i] = leadToResponse(l)
	}

	writeJSON(w, http.StatusOK, listResponse{
		Data:   resp,
		Total:  len(resp),
		Limit:  limit,
		Offset: offset,
	})
}

func (h *LeadHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	workspaceID := middleware.WorkspaceID(ctx)

	l, err := h.q.GetLead(ctx, database.GetLeadParams{
		ID:          id,
		WorkspaceID: workspaceID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "lead not found")
		return
	}

	writeJSON(w, http.StatusOK, leadToResponse(l))
}

func (h *LeadHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceID := middleware.WorkspaceID(ctx)

	var body struct {
		MentionID      *string  `json:"mention_id"`
		Stage          string   `json:"stage"`
		ContactName    *string  `json:"contact_name"`
		ContactEmail   *string  `json:"contact_email"`
		Company        *string  `json:"company"`
		Username       *string  `json:"username"`
		Platform       *string  `json:"platform"`
		ProfileURL     *string  `json:"profile_url"`
		EstimatedValue *int32   `json:"estimated_value"`
		Notes          *string  `json:"notes"`
		Tags           []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if body.Stage == "" {
		body.Stage = "prospect"
	}
	if body.Tags == nil {
		body.Tags = []string{}
	}

	params := database.CreateLeadParams{
		WorkspaceID: workspaceID,
		Stage:       database.LeadStage(body.Stage),
		Tags:        body.Tags,
		Metadata:    []byte("{}"),
	}
	if body.MentionID != nil {
		params.MentionID = parseUUID(*body.MentionID)
	}
	if body.ContactName != nil {
		params.ContactName = pgtype.Text{String: *body.ContactName, Valid: true}
	}
	if body.ContactEmail != nil {
		params.ContactEmail = pgtype.Text{String: *body.ContactEmail, Valid: true}
	}
	if body.Company != nil {
		params.Company = pgtype.Text{String: *body.Company, Valid: true}
	}
	if body.Username != nil {
		params.Username = pgtype.Text{String: *body.Username, Valid: true}
	}
	if body.Platform != nil {
		params.Platform = database.NullPlatformType{
			PlatformType: database.PlatformType(*body.Platform),
			Valid:        true,
		}
	}
	if body.ProfileURL != nil {
		params.ProfileUrl = pgtype.Text{String: *body.ProfileURL, Valid: true}
	}
	if body.EstimatedValue != nil {
		params.EstimatedValue = pgtype.Int4{Int32: *body.EstimatedValue, Valid: true}
	}
	if body.Notes != nil {
		params.Notes = pgtype.Text{String: *body.Notes, Valid: true}
	}

	l, err := h.q.CreateLead(ctx, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create lead")
		return
	}

	writeJSON(w, http.StatusCreated, leadToResponse(l))
}

func (h *LeadHandler) UpdateStage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	workspaceID := middleware.WorkspaceID(ctx)

	var body struct {
		Stage string `json:"stage"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	l, err := h.q.UpdateLeadStage(ctx, database.UpdateLeadStageParams{
		Stage:       database.LeadStage(body.Stage),
		ID:          id,
		WorkspaceID: workspaceID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update lead")
		return
	}

	writeJSON(w, http.StatusOK, leadToResponse(l))
}

func (h *LeadHandler) Counts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceID := middleware.WorkspaceID(ctx)

	counts, err := h.q.CountLeadsByStage(ctx, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count leads")
		return
	}

	type countItem struct {
		Status string `json:"status"`
		Count  int32  `json:"count"`
	}
	resp := make([]countItem, len(counts))
	for i, c := range counts {
		resp[i] = countItem{Status: string(c.Stage), Count: c.Count}
	}

	writeJSON(w, http.StatusOK, resp)
}

func parseUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if len(s) != 36 {
		return u
	}
	// Parse hex pairs, skipping dashes
	hex := func(c byte) byte {
		switch {
		case '0' <= c && c <= '9':
			return c - '0'
		case 'a' <= c && c <= 'f':
			return c - 'a' + 10
		case 'A' <= c && c <= 'F':
			return c - 'A' + 10
		}
		return 0
	}
	src := s
	dst := 0
	for i := 0; i < len(src); i++ {
		if src[i] == '-' {
			continue
		}
		if i+1 >= len(src) {
			return u
		}
		u.Bytes[dst] = hex(src[i])<<4 | hex(src[i+1])
		dst++
		i++
	}
	u.Valid = true
	return u
}
