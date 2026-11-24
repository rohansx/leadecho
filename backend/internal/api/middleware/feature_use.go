package middleware

import (
	"context"
	"net/http"

	"github.com/rs/zerolog"
)

// FeatureMarker records that a workspace used a feature (*database.Queries).
type FeatureMarker interface {
	MarkFeatureUsed(ctx context.Context, workspaceID, feature string) error
}

// FeatureUse marks the feature as used by the request's workspace before
// serving the page's data. Background jobs for hidden pages (demand,
// competitor watch) only run for workspaces that opened the page lately.
// A failed mark is logged; the request is served anyway.
func FeatureUse(m FeatureMarker, feature string, logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws := WorkspaceID(r.Context())
			if err := m.MarkFeatureUsed(r.Context(), ws, feature); err != nil {
				logger.Warn().Err(err).Str("workspace_id", ws).Str("feature", feature).Msg("feature use: mark failed")
			}
			next.ServeHTTP(w, r)
		})
	}
}
