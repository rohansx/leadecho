package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"leadecho/internal/auth"
	"leadecho/internal/config"
	"leadecho/internal/database"
)

// registerAuthRoutes mounts built-in auth: email/password (with verification
// and reset), and Google/GitHub OAuth when their credentials are configured.
// It returns the session-version checker the protected-route middleware uses.
func registerAuthRoutes(r chi.Router, cfg *config.Config, q *database.Queries, redis *goredis.Client, logger zerolog.Logger) *auth.SessionVersions {
	versions := auth.NewSessionVersions(q)
	var counter auth.Counter
	if redis != nil {
		counter = auth.NewRedisCounter(redis)
	}
	email := auth.NewEmailHandler(auth.Config{
		JWTSecret:           cfg.JWTSecret,
		FrontendURL:         cfg.FrontendURL,
		ResendAPIKey:        cfg.ResendAPIKey,
		RequireVerification: cfg.RequireEmailVerification(),
		SecureCookie:        cfg.SecureCookies(),
		TrustedProxyHops:    cfg.TrustedProxyHops,
		FromEmail:           cfg.DigestFromEmail, // one verified sender for all mail
	}, q, counter, versions, logger)
	if !email.RequiresVerification() {
		logger.Warn().Msg("RESEND_API_KEY not set: email verification and password-reset emails are disabled; new password accounts are auto-verified")
	}

	r.Post("/auth/register", email.Register)
	r.Post("/auth/login", email.Login)
	r.Post("/auth/logout", email.Logout)
	r.Get("/auth/me", email.Me)
	r.Get("/auth/verify", email.Verify)
	r.Post("/auth/resend-verification", email.ResendVerification)
	r.Post("/auth/forgot-password", email.ForgotPassword)
	r.Post("/auth/reset-password", email.ResetPassword)

	front := strings.TrimRight(cfg.FrontendURL, "/")
	redirect := func(set, provider string) string {
		if set != "" {
			return set
		}
		return front + "/api/v1/auth/" + provider + "/callback"
	}
	var google, github, logto bool
	if cfg.LogtoEndpoint != "" && cfg.LogtoAppID != "" && cfg.LogtoAppSecret != "" {
		h := auth.NewOAuthHandler(auth.NewLogtoProvider(cfg.LogtoEndpoint, cfg.LogtoAppID, cfg.LogtoAppSecret, redirect(cfg.LogtoRedirectURL, "logto")), email)
		r.Get("/auth/logto", h.Login)
		r.Get("/auth/logto/callback", h.Callback)
		logto = true
	}
	if cfg.GoogleClientID != "" && cfg.GoogleClientSecret != "" {
		h := auth.NewOAuthHandler(auth.NewGoogleProvider(cfg.GoogleClientID, cfg.GoogleClientSecret, redirect(cfg.GoogleRedirectURL, "google")), email)
		r.Get("/auth/google", h.Login)
		r.Get("/auth/google/callback", h.Callback)
		google = true
	}
	if cfg.GitHubClientID != "" && cfg.GitHubClientSecret != "" {
		h := auth.NewOAuthHandler(auth.NewGitHubProvider(cfg.GitHubClientID, cfg.GitHubClientSecret, redirect(cfg.GitHubRedirectURL, "github")), email)
		r.Get("/auth/github", h.Login)
		r.Get("/auth/github/callback", h.Callback)
		github = true
	}

	providers := fmt.Sprintf(`{"google":%t,"github":%t,"logto":%t,"password":true,"email_verification":%t}`, google, github, logto, email.RequiresVerification())
	r.Get("/auth/providers", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(providers))
	})
	return versions
}
