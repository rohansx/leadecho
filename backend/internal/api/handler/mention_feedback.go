package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

// scoring: 👍/👎 feedback on mentions, used as few-shot examples per listener.

const maxFeedbackNote = 500

type feedbackBody struct {
	Label string `json:"label"` // up | down
	Note  string `json:"note"`
}

type feedbackResponse struct {
	Label     string  `json:"label"`
	Note      string  `json:"note"`
	ProfileID *string `json:"profile_id"`
}

func parseFeedbackBody(r io.Reader) (feedbackBody, error) {
	var b feedbackBody
	if err := json.NewDecoder(io.LimitReader(r, 64<<10)).Decode(&b); err != nil {
		return b, errors.New("invalid JSON")
	}
	b.Label = strings.ToLower(strings.TrimSpace(b.Label))
	b.Note = strings.TrimSpace(b.Note)
	if b.Label != "up" && b.Label != "down" {
		return b, errors.New("label must be \"up\" or \"down\"")
	}
	if len(b.Note) > maxFeedbackNote {
		return b, errors.New("note must be <= 500 characters")
	}
	return b, nil
}

// Feedback records the caller's 👍/👎 on a mention (latest vote wins).
// POST /mentions/{id}/feedback {"label":"up"|"down","note":"..."}
func (h *MentionHandler) Feedback(w http.ResponseWriter, r *http.Request) {
	h.feedbackFor(w, r, middleware.WorkspaceID(r.Context()), middleware.UserID(r.Context()))
}

func (h *MentionHandler) feedbackFor(w http.ResponseWriter, r *http.Request, wsID, userID string) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if !parseUUID(id).Valid {
		writeError(w, http.StatusBadRequest, "invalid mention id")
		return
	}
	body, err := parseFeedbackBody(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	mention, err := h.q.GetMention(ctx, database.GetMentionParams{ID: id, WorkspaceID: wsID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "mention not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load mention")
		return
	}

	// The listener is derived server-side from the mention's keyword.
	var profileID pgtype.UUID
	prof, err := h.q.GetMentionScoringProfile(ctx, database.GetMentionScoringProfileParams{MentionID: id, WorkspaceID: wsID})
	switch {
	case err == nil:
		profileID = parseUUID(prof.ID)
	case !errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusInternalServerError, "failed to load listener")
		return
	}

	fb, err := h.q.UpsertMentionFeedback(ctx, database.UpsertMentionFeedbackParams{
		WorkspaceID: wsID,
		MentionID:   id,
		ProfileID:   profileID,
		UserID:      parseUUID(userID),
		Label:       body.Label,
		Note:        body.Note,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record feedback")
		return
	}

	updated, err := h.mirrorFeedbackToMetadata(r, mention, body, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record feedback")
		return
	}

	resp := feedbackResponse{Label: fb.Label, Note: fb.Note}
	if fb.ProfileID.Valid {
		s := uuidToString(fb.ProfileID)
		resp.ProfileID = &s
	}
	writeJSON(w, http.StatusOK, map[string]any{"feedback": resp, "mention": mentionToResponse(updated)})
}

// mirrorFeedbackToMetadata keeps scoring_metadata.last_feedback /
// feedback_history in step (precision analytics and the dashboard read it).
func (h *MentionHandler) mirrorFeedbackToMetadata(r *http.Request, m database.Mention, body feedbackBody, userID string) (database.Mention, error) {
	var prevScore *float32
	if m.RelevanceScore.Valid {
		v := m.RelevanceScore.Float32
		prevScore = &v
	}
	prevIntent := ""
	if m.Intent.Valid {
		prevIntent = string(m.Intent.IntentType)
	}
	meta, err := mergeScoringFeedback(m.ScoringMetadata, body.Label, body.Note, userID, prevScore, prevIntent)
	if err != nil {
		return m, err
	}
	return h.q.PatchMentionScoringMetadata(r.Context(), database.PatchMentionScoringMetadataParams{
		ScoringMetadata: meta, ID: m.ID, WorkspaceID: m.WorkspaceID,
	})
}

func nonNilTags(t []string) []string {
	if t == nil {
		return []string{}
	}
	return t
}

// validateTagFilters checks the tag / sentiment list filters ("" = valid).
func validateTagFilters(p database.ListMentionsComposedParams) string {
	if p.Tag != "" && !ai.IsIntentTag(p.Tag) {
		return "invalid tag"
	}
	if p.Sentiment != "" && !ai.IsSentiment(p.Sentiment) {
		return "invalid sentiment"
	}
	return ""
}
