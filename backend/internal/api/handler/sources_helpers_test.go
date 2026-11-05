package handler

import (
	"net/http"

	"leadecho/internal/api/middleware"
	"leadecho/internal/auth"
)

func withWorkspace(r *http.Request, ws string) *http.Request {
	return r.WithContext(middleware.WithClaims(r.Context(), &auth.Claims{WorkspaceID: ws, UserID: "11111111-1111-1111-1111-111111111111"}))
}
