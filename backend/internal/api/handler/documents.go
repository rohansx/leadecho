package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
	"leadecho/internal/knowledge"
)

type DocumentHandler struct {
	q      *database.Queries
	kb     *knowledge.Service
	logger zerolog.Logger
}

func NewDocumentHandler(q *database.Queries, kb *knowledge.Service, logger zerolog.Logger) *DocumentHandler {
	return &DocumentHandler{q: q, kb: kb, logger: logger}
}

type DocumentResponse struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspace_id"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	ContentType   string `json:"content_type"`
	SourceURL     string `json:"source_url,omitempty"`
	FileSizeBytes int32  `json:"file_size_bytes,omitempty"`
	ChunkCount    int32  `json:"chunk_count"`
	IsActive      bool   `json:"is_active"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func docToResponse(d database.Document) DocumentResponse {
	r := DocumentResponse{
		ID:          d.ID,
		WorkspaceID: d.WorkspaceID,
		Title:       d.Title,
		Content:     d.Content,
		ContentType: d.ContentType,
		ChunkCount:  d.ChunkCount,
		IsActive:    d.IsActive,
		CreatedAt:   d.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   d.UpdatedAt.Format(time.RFC3339),
	}
	if d.SourceUrl.Valid {
		r.SourceURL = d.SourceUrl.String
	}
	if d.FileSizeBytes.Valid {
		r.FileSizeBytes = d.FileSizeBytes.Int32
	}
	return r
}

func (h *DocumentHandler) indexDocument(ctx context.Context, wsID string, doc database.Document) {
	if h.kb == nil {
		return
	}
	if err := h.kb.IndexDocument(ctx, wsID, doc.ID, doc.Title, doc.Content); err != nil {
		h.logger.Warn().Err(err).Str("document_id", doc.ID).Msg("knowledge index failed")
	}
}

func (h *DocumentHandler) List(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	docs, err := h.q.ListDocuments(r.Context(), wsID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list documents")
		return
	}
	resp := make([]DocumentResponse, len(docs))
	for i, d := range docs {
		resp[i] = docToResponse(d)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *DocumentHandler) Get(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	id := chi.URLParam(r, "id")
	d, err := h.q.GetDocument(r.Context(), database.GetDocumentParams{ID: id, WorkspaceID: wsID})
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	writeJSON(w, http.StatusOK, docToResponse(d))
}

func (h *DocumentHandler) Create(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	ctx := r.Context()

	var body struct {
		Title       string `json:"title"`
		Content     string `json:"content"`
		ContentType string `json:"content_type"`
		SourceURL   string `json:"source_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Title == "" || body.Content == "" {
		writeError(w, http.StatusBadRequest, "title and content are required")
		return
	}
	if body.ContentType == "" {
		body.ContentType = "markdown"
	}
	if body.SourceURL != "" && !isHTTPURL(body.SourceURL) {
		writeError(w, http.StatusBadRequest, "source_url must be an http(s) URL")
		return
	}

	params := database.CreateDocumentParams{
		WorkspaceID: wsID,
		Title:       body.Title,
		Content:     body.Content,
		ContentType: body.ContentType,
	}
	if body.SourceURL != "" {
		params.SourceUrl = pgtype.Text{String: body.SourceURL, Valid: true}
	}
	params.FileSizeBytes = pgtype.Int4{Int32: int32(len(body.Content)), Valid: true}

	d, err := h.q.CreateDocument(ctx, params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create document")
		return
	}

	h.indexDocument(ctx, wsID, d)

	updated, err := h.q.GetDocument(ctx, database.GetDocumentParams{ID: d.ID, WorkspaceID: wsID})
	if err == nil {
		d = updated
	}
	writeJSON(w, http.StatusCreated, docToResponse(d))
}

func (h *DocumentHandler) Update(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var body struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	existing, err := h.q.GetDocument(ctx, database.GetDocumentParams{ID: id, WorkspaceID: wsID})
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}

	title := existing.Title
	if body.Title != "" {
		title = body.Title
	}
	content := existing.Content
	if body.Content != "" {
		content = body.Content
	}

	d, err := h.q.UpdateDocument(ctx, database.UpdateDocumentParams{
		ID:          id,
		WorkspaceID: wsID,
		Title:       title,
		Content:     content,
		IsActive:    existing.IsActive,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update document")
		return
	}

	h.indexDocument(ctx, wsID, d)

	updated, err := h.q.GetDocument(ctx, database.GetDocumentParams{ID: d.ID, WorkspaceID: wsID})
	if err == nil {
		d = updated
	}
	writeJSON(w, http.StatusOK, docToResponse(d))
}

func (h *DocumentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	wsID := middleware.WorkspaceID(r.Context())
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if err := h.q.DeleteDocument(ctx, database.DeleteDocumentParams{ID: id, WorkspaceID: wsID}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete document")
		return
	}
	if h.kb != nil {
		_ = h.kb.DeleteDocument(ctx, wsID, id)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// isHTTPURL reports whether s is an absolute http(s) URL with a host.
func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
