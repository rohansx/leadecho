package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"leadecho/internal/clientip"
)

// Logger logs each request. trustedProxyHops controls client-IP resolution
// (see clientip.From); spoofable headers are never trusted by default.
func Logger(logger zerolog.Logger, trustedProxyHops int) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.Status()).
				Dur("duration", time.Since(start)).
				Str("remote_addr", clientip.From(r, trustedProxyHops)).
				Msg("request")
		})
	}
}
