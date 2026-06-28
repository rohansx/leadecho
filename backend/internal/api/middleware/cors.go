package middleware

import (
	"net/http"
	"strings"

	"github.com/go-chi/cors"
)

func CORSOptions(frontendURL string) cors.Options {
	return cors.Options{
		// Allow the dashboard origin and the Chrome extension (chrome-extension://<id>).
		// Reflecting the specific origin (rather than "*") keeps AllowCredentials valid.
		AllowOriginFunc: func(_ *http.Request, origin string) bool {
			if frontendURL != "" && origin == frontendURL {
				return true
			}
			return strings.HasPrefix(origin, "chrome-extension://")
		},
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-Workspace-ID", "X-Extension-Key"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}
}
