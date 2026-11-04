package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
	"leadecho/internal/reply"
)

// ResponseProfileHandler serves /response-profiles (reply voice).
type ResponseProfileHandler struct {
	q *database.Queries
}

func NewResponseProfileHandler(q *database.Queries) *ResponseProfileHandler {
	return &ResponseProfileHandler{q: q}
}

const maxProfileNameLen = 120

type responseProfileResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	reply.Voice
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func toResponseProfileResponse(p database.ResponseProfile) responseProfileResponse {
	return responseProfileResponse{
		ID: p.ID, Name: p.Name, IsDefault: p.IsDefault, Voice: reply.VoiceFromProfile(p),
		CreatedAt: p.CreatedAt.Format(time.RFC3339), UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
	}
}

// decodeResponseProfile reads a profile body over base (defaults on create,
// the stored profile on update), so omitted fields keep their value.
func decodeResponseProfile(r *http.Request, name string, base reply.Voice) (string, reply.Voice, error) {
	body := struct {
		Name *string `json:"name"`
		reply.Voice
	}{Voice: base}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); err != nil {
		return "", reply.Voice{}, errors.New("invalid JSON")
	}
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
	}
	if name == "" || len(name) > maxProfileNameLen {
		return "", reply.Voice{}, errors.New("name is required (max 120 characters)")
	}
	if body.Examples == nil {
		body.Examples = map[string]string{}
	}
	body.DestinationURL = strings.TrimSpace(body.DestinationURL)
	if err := reply.ValidateVoice(body.Voice); err != nil {
		return "", reply.Voice{}, err
	}
	return name, body.Voice, nil
}

// List — GET /response-profiles
func (h *ResponseProfileHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListResponseProfiles(r.Context(), middleware.WorkspaceID(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list response profiles")
		return
	}
	out := make([]responseProfileResponse, len(rows))
	for i, p := range rows {
		out[i] = toResponseProfileResponse(p)
	}
	writeJSON(w, http.StatusOK, out)
}

// Create — POST /response-profiles (the first one becomes the default)
func (h *ResponseProfileHandler) Create(w http.ResponseWriter, r *http.Request) {
	name, v, err := decodeResponseProfile(r, "", reply.DefaultVoice())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ex, _ := json.Marshal(v.Examples)
	p, err := h.q.CreateResponseProfile(r.Context(), database.CreateResponseProfileParams{
		WorkspaceID: middleware.WorkspaceID(r.Context()), Name: name, Goal: v.Goal,
		SpecialInstructions: v.SpecialInstructions, Tone: int32(v.Tone), Length: v.Length,
		Assertiveness: int32(v.Assertiveness), BrandPlug: v.BrandPlug, CtaGoal: v.CTAGoal,
		DestinationUrl: v.DestinationURL, Examples: ex,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create response profile")
		return
	}
	writeJSON(w, http.StatusCreated, toResponseProfileResponse(p))
}

// Update — PUT /response-profiles/{id} (omitted fields keep their value)
func (h *ResponseProfileHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx, wsID, id := r.Context(), middleware.WorkspaceID(r.Context()), chi.URLParam(r, "id")
	if !parseUUID(id).Valid {
		writeError(w, http.StatusNotFound, "response profile not found")
		return
	}
	existing, err := h.q.GetResponseProfile(ctx, database.GetResponseProfileParams{ID: id, WorkspaceID: wsID})
	if err != nil {
		h.notFoundOr500(w, err)
		return
	}
	name, v, err := decodeResponseProfile(r, existing.Name, reply.VoiceFromProfile(existing))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ex, _ := json.Marshal(v.Examples)
	p, err := h.q.UpdateResponseProfile(ctx, database.UpdateResponseProfileParams{
		ID: id, WorkspaceID: wsID, Name: name, Goal: v.Goal,
		SpecialInstructions: v.SpecialInstructions, Tone: int32(v.Tone), Length: v.Length,
		Assertiveness: int32(v.Assertiveness), BrandPlug: v.BrandPlug, CtaGoal: v.CTAGoal,
		DestinationUrl: v.DestinationURL, Examples: ex,
	})
	if err != nil {
		h.notFoundOr500(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponseProfileResponse(p))
}

// Delete — DELETE /response-profiles/{id}; deleting the default promotes the oldest.
func (h *ResponseProfileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx, wsID, id := r.Context(), middleware.WorkspaceID(r.Context()), chi.URLParam(r, "id")
	if !parseUUID(id).Valid {
		writeError(w, http.StatusNotFound, "response profile not found")
		return
	}
	n, err := h.q.DeleteResponseProfile(ctx, database.DeleteResponseProfileParams{ID: id, WorkspaceID: wsID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete response profile")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "response profile not found")
		return
	}
	if next, err := h.q.GetPreferredResponseProfile(ctx, wsID); err == nil && !next.IsDefault {
		_, _ = h.q.SetDefaultResponseProfile(ctx, database.SetDefaultResponseProfileParams{ID: next.ID, WorkspaceID: wsID})
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// SetDefault — POST /response-profiles/{id}/default
func (h *ResponseProfileHandler) SetDefault(w http.ResponseWriter, r *http.Request) {
	ctx, wsID, id := r.Context(), middleware.WorkspaceID(r.Context()), chi.URLParam(r, "id")
	if !parseUUID(id).Valid {
		writeError(w, http.StatusNotFound, "response profile not found")
		return
	}
	n, err := h.q.SetDefaultResponseProfile(ctx, database.SetDefaultResponseProfileParams{ID: id, WorkspaceID: wsID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set default response profile")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "response profile not found")
		return
	}
	p, err := h.q.GetResponseProfile(ctx, database.GetResponseProfileParams{ID: id, WorkspaceID: wsID})
	if err != nil {
		h.notFoundOr500(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponseProfileResponse(p))
}

func (h *ResponseProfileHandler) notFoundOr500(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "response profile not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "failed to load response profile")
}

// draftReplyProfileID reads the optional {"response_profile_id"} draft-reply
// body. An empty body is fine — older clients send none.
func draftReplyProfileID(r *http.Request) (string, error) {
	var body struct {
		ResponseProfileID string `json:"response_profile_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		return "", errors.New("invalid JSON")
	}
	id := strings.TrimSpace(body.ResponseProfileID)
	if id != "" && !parseUUID(id).Valid {
		return "", errors.New("response_profile_id must be a UUID")
	}
	return id, nil
}
