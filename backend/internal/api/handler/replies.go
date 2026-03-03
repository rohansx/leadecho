package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

type ReplyHandler struct {
	q *database.Queries
}

func NewReplyHandler(q *database.Queries) *ReplyHandler {
	return &ReplyHandler{q: q}
}

type ReplyResponse struct {
	ID            string  `json:"id"`
	MentionID     string  `json:"mention_id"`
	WorkspaceID   string  `json:"workspace_id"`
	Content       string  `json:"content"`
	EditedContent *string `json:"edited_content"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

func replyToResponse(r database.Reply) ReplyResponse {
	resp := ReplyResponse{
		ID:          r.ID,
		MentionID:   r.MentionID,
		WorkspaceID: r.WorkspaceID,
		Content:     r.Content,
		Status:      string(r.Status),
		CreatedAt:   r.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   r.UpdatedAt.Format(time.RFC3339),
	}
	if r.EditedContent.Valid {
		resp.EditedContent = &r.EditedContent.String
	}
	return resp
}

func (h *ReplyHandler) ListByMention(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	mentionID := chi.URLParam(r, "mentionId")

	replies, err := h.q.ListRepliesByMention(r.Context(), database.ListRepliesByMentionParams{
		MentionID:   mentionID,
		WorkspaceID: wsID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list replies")
		return
	}
	resp := make([]ReplyResponse, len(replies))
	for i, rp := range replies {
		resp[i] = replyToResponse(rp)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *ReplyHandler) Create(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())

	var body struct {
		MentionID string `json:"mention_id"`
		Content   string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.MentionID == "" || body.Content == "" {
		writeError(w, http.StatusBadRequest, "mention_id and content are required")
		return
	}

	rp, err := h.q.CreateReply(r.Context(), database.CreateReplyParams{
		MentionID:   body.MentionID,
		WorkspaceID: wsID,
		Content:     body.Content,
		Status:      database.ReplyStatusDraft,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create reply")
		return
	}
	writeJSON(w, http.StatusCreated, replyToResponse(rp))
}

func (h *ReplyHandler) UpdateContent(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	id := chi.URLParam(r, "id")

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	rp, err := h.q.UpdateReplyContent(r.Context(), database.UpdateReplyContentParams{
		ID:            id,
		WorkspaceID:   wsID,
		EditedContent: pgtype.Text{String: body.Content, Valid: true},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update reply")
		return
	}
	writeJSON(w, http.StatusOK, replyToResponse(rp))
}

func (h *ReplyHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	id := chi.URLParam(r, "id")

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	rp, err := h.q.UpdateReplyStatus(r.Context(), database.UpdateReplyStatusParams{
		ID:          id,
		WorkspaceID: wsID,
		Status:      database.ReplyStatus(body.Status),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update reply status")
		return
	}
	writeJSON(w, http.StatusOK, replyToResponse(rp))
}
