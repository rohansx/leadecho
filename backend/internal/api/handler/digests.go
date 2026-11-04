package handler

import (
	"encoding/json"
	"net/http"

	"leadecho/internal/api/middleware"
	"leadecho/internal/digest"
)

// DigestHandler serves the daily/weekly digest settings and preview (P1 #13).
type DigestHandler struct {
	svc *digest.Service
}

func NewDigestHandler(svc *digest.Service) *DigestHandler {
	return &DigestHandler{svc: svc}
}

func (h *DigestHandler) Get(w http.ResponseWriter, r *http.Request) {
	sched, err := h.svc.Settings(r.Context(), middleware.WorkspaceID(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load digest settings")
		return
	}
	writeJSON(w, http.StatusOK, sched)
}

func (h *DigestHandler) Put(w http.ResponseWriter, r *http.Request) {
	var in digest.Schedule
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := in.Normalize(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sched, err := h.svc.SaveSettings(r.Context(), middleware.WorkspaceID(r.Context()), in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save digest settings")
		return
	}
	writeJSON(w, http.StatusOK, sched)
}

// Preview renders the digest for the trailing day/week as HTML (not sent).
func (h *DigestHandler) Preview(w http.ResponseWriter, r *http.Request) {
	kind := digest.Kind(r.URL.Query().Get("kind"))
	if kind == "" {
		kind = digest.KindDaily
	}
	if kind != digest.KindDaily && kind != digest.KindWeekly {
		writeError(w, http.StatusBadRequest, "kind must be daily or weekly")
		return
	}
	html, err := h.svc.Preview(r.Context(), middleware.WorkspaceID(r.Context()), kind)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build digest preview")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}
