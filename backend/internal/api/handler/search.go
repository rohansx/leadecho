package handler

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
)

// searcher is the slice of *database.Queries the search needs (fakeable).
type searcher interface {
	SearchPeople(ctx context.Context, workspaceID, query string, limit int32) ([]database.SearchPerson, error)
	SearchPosts(ctx context.Context, workspaceID, query string, limit int32) ([]database.SearchPost, error)
}

// SearchHandler serves the dashboard's global search.
type SearchHandler struct{ q searcher }

func NewSearchHandler(q searcher) *SearchHandler { return &SearchHandler{q: q} }

const (
	searchMinLen = 2
	searchMaxLen = 100
	searchLimit  = 8
)

// SearchResult groups matches by what they are.
type SearchResult struct {
	People []database.SearchPerson `json:"people"`
	Posts  []database.SearchPost   `json:"posts"`
}

// Search: GET /search?q= . People and posts in the caller's workspace.
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if n := utf8.RuneCountInString(q); n < searchMinLen || n > searchMaxLen {
		writeError(w, http.StatusBadRequest, "search text must be 2 to 100 characters")
		return
	}
	ws := middleware.WorkspaceID(r.Context())
	people, err := h.q.SearchPeople(r.Context(), ws, q, searchLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	posts, err := h.q.SearchPosts(r.Context(), ws, q, searchLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	writeJSON(w, http.StatusOK, SearchResult{People: people, Posts: posts})
}
