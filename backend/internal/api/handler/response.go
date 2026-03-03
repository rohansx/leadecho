package handler

import (
	"encoding/json"
	"net/http"
)

type listResponse struct {
	Data   any   `json:"data"`
	Total  int   `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
