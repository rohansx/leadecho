package middleware

import (
	"context"
	"net/http"

	"leadecho/internal/database"
)

type extensionWorkspaceKey struct{}

// ExtensionKeyAuth reads X-Extension-Key, validates it against the DB,
// and injects the workspace ID into the request context.
// TouchExtensionToken runs in a goroutine to avoid blocking the request.
func ExtensionKeyAuth(q *database.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("X-Extension-Key")
			if token == "" {
				http.Error(w, `{"error":"missing X-Extension-Key"}`, http.StatusUnauthorized)
				return
			}

			row, err := q.GetExtensionTokenByToken(r.Context(), token)
			if err != nil {
				http.Error(w, `{"error":"invalid extension key"}`, http.StatusUnauthorized)
				return
			}

			go func() {
				_ = q.TouchExtensionToken(context.Background(), token)
			}()

			ctx := context.WithValue(r.Context(), extensionWorkspaceKey{}, row.WorkspaceID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ExtensionWorkspaceID extracts the workspace ID injected by ExtensionKeyAuth.
func ExtensionWorkspaceID(ctx context.Context) string {
	if v, ok := ctx.Value(extensionWorkspaceKey{}).(string); ok {
		return v
	}
	return ""
}
