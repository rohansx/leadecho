package middleware

import (
	"context"
	"net/http"

	"leadecho/internal/auth"
)

type contextKey string

const claimsKey contextKey = "claims"

// Auth validates the session JWT from the cookie and injects claims into context.
func Auth(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session")
			if err != nil {
				http.Error(w, `{"error":"not authenticated"}`, http.StatusUnauthorized)
				return
			}

			claims, err := auth.ValidateToken(jwtSecret, cookie.Value)
			if err != nil {
				http.Error(w, `{"error":"invalid session"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsFromContext extracts the JWT claims from the request context.
func ClaimsFromContext(ctx context.Context) *auth.Claims {
	claims, _ := ctx.Value(claimsKey).(*auth.Claims)
	return claims
}

// WorkspaceID extracts the workspace ID from the request context.
// Falls back to the dev workspace ID if no auth is present.
func WorkspaceID(ctx context.Context) string {
	claims := ClaimsFromContext(ctx)
	if claims != nil {
		return claims.WorkspaceID
	}
	return "00000000-0000-0000-0000-000000000001"
}
