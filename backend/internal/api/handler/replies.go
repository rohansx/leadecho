package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
	"leadecho/internal/events"
	"leadecho/internal/events/publishers"
)

type ReplyHandler struct {
	q          *database.Queries
	publisher  *publishers.Publisher
	streamsOn  bool
	publicBase string // e.g. http://localhost:8090 — builds /r/{code} short links
}

func NewReplyHandler(q *database.Queries, publisher *publishers.Publisher, streamsOn bool, publicBase string) *ReplyHandler {
	return &ReplyHandler{q: q, publisher: publisher, streamsOn: streamsOn, publicBase: strings.TrimRight(publicBase, "/")}
}

type ReplyResponse struct {
	ID            string  `json:"id"`
	MentionID     string  `json:"mention_id"`
	WorkspaceID   string  `json:"workspace_id"`
	Content       string  `json:"content"`
	EditedContent *string `json:"edited_content"`
	Status        string  `json:"status"`
	UTMLinkID     *string `json:"utm_link_id,omitempty"`
	UTMCode       *string `json:"utm_code,omitempty"`
	ShortURL      *string `json:"short_url,omitempty"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

func (h *ReplyHandler) replyToResponse(ctx context.Context, r database.Reply) ReplyResponse {
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
	if r.UtmLinkID.Valid {
		id := formatUUID(r.UtmLinkID)
		resp.UTMLinkID = &id
		if link, err := h.q.GetUTMLinkByID(ctx, database.GetUTMLinkByIDParams{
			ID:          id,
			WorkspaceID: r.WorkspaceID,
		}); err == nil {
			code := link.Code
			resp.UTMCode = &code
			short := h.shortURL(code)
			resp.ShortURL = &short
		}
	}
	return resp
}

func (h *ReplyHandler) shortURL(code string) string {
	if h.publicBase != "" {
		return h.publicBase + "/r/" + code
	}
	return "/r/" + code
}

func basicReplyResponse(r database.Reply) ReplyResponse {
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

func formatUUID(u pgtype.UUID) string {
	b := u.Bytes
	return fmt.Sprintf(
		"%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3], b[4], b[5], b[6], b[7],
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15],
	)
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
		resp[i] = h.replyToResponse(r.Context(), rp)
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
	writeJSON(w, http.StatusCreated, h.replyToResponse(r.Context(), rp))
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
	writeJSON(w, http.StatusOK, h.replyToResponse(r.Context(), rp))
}

func (h *ReplyHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	id := chi.URLParam(r, "id")

	var body struct {
		Status         string `json:"status"`
		DestinationURL string `json:"destination_url"`
		AppendShortURL *bool  `json:"append_short_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	switch body.Status {
	case "approved":
		appendShort := body.AppendShortURL == nil || *body.AppendShortURL
		h.approve(w, r, wsID, id, body.DestinationURL, appendShort)
	case "draft", "posted", "failed":
		rp, err := h.q.UpdateReplyStatus(r.Context(), database.UpdateReplyStatusParams{
			ID:          id,
			WorkspaceID: wsID,
			Status:      database.ReplyStatus(body.Status),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update reply status")
			return
		}
		writeJSON(w, http.StatusOK, h.replyToResponse(r.Context(), rp))
	default:
		writeError(w, http.StatusBadRequest, "invalid status")
	}
}

func (h *ReplyHandler) approve(w http.ResponseWriter, r *http.Request, wsID, id, destinationURL string, appendShort bool) {
	ctx := r.Context()
	existing, err := h.q.GetReply(ctx, database.GetReplyParams{ID: id, WorkspaceID: wsID})
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "reply not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load reply")
		return
	}

	utmID := existing.UtmLinkID
	edited := existing.EditedContent

	if destinationURL != "" {
		if !isHTTPURL(destinationURL) {
			writeError(w, http.StatusBadRequest, "destination_url must be an http(s) URL")
			return
		}
		var link database.UtmLink
		created := false
		for attempts := 0; attempts < 5; attempts++ {
			code := randomCode(8)
			link, err = h.q.CreateUTMLink(ctx, database.CreateUTMLinkParams{
				WorkspaceID:    wsID,
				Code:           code,
				DestinationUrl: destinationURL,
				UtmSource:      "leadecho",
				UtmMedium:      "social_reply",
				UtmCampaign:    pgtype.Text{String: existing.MentionID, Valid: true},
				UtmContent:     pgtype.Text{String: existing.ID, Valid: true},
			})
			if err == nil {
				created = true
				break
			}
			if !isPgUniqueViolation(err) {
				writeError(w, http.StatusInternalServerError, "failed to create UTM link")
				return
			}
		}
		if !created {
			writeError(w, http.StatusInternalServerError, "failed to create UTM link")
			return
		}
		utmID = parseUUID(link.ID)
		if appendShort {
			base := existing.Content
			if existing.EditedContent.Valid && existing.EditedContent.String != "" {
				base = existing.EditedContent.String
			}
			short := h.shortURL(link.Code)
			if !strings.Contains(base, link.Code) {
				base = strings.TrimRight(base, "\n") + "\n\n" + short
			}
			edited = pgtype.Text{String: base, Valid: true}
		}
	}

	rp, err := h.q.ApproveReply(ctx, database.ApproveReplyParams{
		ID:            id,
		WorkspaceID:   wsID,
		ApprovedBy:    parseUUID(middleware.UserID(ctx)),
		UtmLinkID:     utmID,
		EditedContent: edited,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to approve reply")
		return
	}

	if h.streamsOn && h.publisher != nil {
		env, err := events.NewEnvelope(
			events.EventTypeReplyApproved,
			events.AggregateTypeReply,
			rp.ID,
			"api",
			wsID,
			fmt.Sprintf("%s:%s", events.EventTypeReplyApproved, rp.ID),
			events.ReplyApprovedPayload{
				ReplyID:     rp.ID,
				MentionID:   rp.MentionID,
				WorkspaceID: wsID,
			},
		)
		if err == nil {
			_, _ = h.publisher.Publish(context.Background(), env)
		}
	}

	writeJSON(w, http.StatusOK, h.replyToResponse(ctx, rp))
}
