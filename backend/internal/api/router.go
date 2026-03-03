package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"leadecho/internal/api/handler"
	"leadecho/internal/api/middleware"
	"leadecho/internal/auth"
	"leadecho/internal/browser"
	"leadecho/internal/config"
	"leadecho/internal/crypto"
	"leadecho/internal/database"
	"leadecho/internal/embedding"
	"leadecho/internal/monitor"
)

func NewRouter(logger zerolog.Logger, db *pgxpool.Pool, redis *goredis.Client, cfg *config.Config, embedder *embedding.Client, pinchtab *browser.PinchtabClient, mon *monitor.Monitor) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger(logger))
	r.Use(chimiddleware.Recoverer)
	r.Use(cors.Handler(middleware.CORSOptions(cfg.FrontendURL)))

	// Health checks (no auth)
	health := handler.NewHealthHandler(db, redis)
	r.Get("/healthz", health.Healthz)
	r.Get("/readyz", health.Readyz)

	// sqlc queries instance
	queries := database.New(db)

	// Public UTM redirect — outside /api/v1, no auth
	utmPublic := handler.NewUTMHandler(queries)
	r.Get("/r/{code}", utmPublic.RedirectUTM)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"message":"pong"}`))
		})

		// Email auth routes (always available)
		email := auth.NewEmailHandler(cfg.JWTSecret, cfg.ResendAPIKey, queries, logger)
		r.Post("/auth/register", email.Register)
		r.Post("/auth/login", email.Login)
		r.Get("/auth/me", email.Me)
		r.Post("/auth/logout", email.Logout)

		// Google OAuth routes (only when configured)
		if cfg.GoogleClientID != "" {
			google := auth.NewGoogleHandler(
				cfg.GoogleClientID,
				cfg.GoogleClientSecret,
				cfg.GoogleRedirectURL,
				cfg.JWTSecret,
				cfg.FrontendURL,
				queries,
				logger,
			)
			r.Get("/auth/google", google.Login)
			r.Get("/auth/google/callback", google.Callback)
		}

		// Encryption key for BYOK API keys
		encKey := crypto.DeriveKey(cfg.EncryptionKeyOrDefault())

		// Extension handler (used in both JWT group and extension-key group below)
		ext := handler.NewExtensionHandler(queries, mon)

		// Protected API routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(cfg.JWTSecret))

			// Mentions
			mentions := handler.NewMentionHandler(queries)
			r.Get("/mentions", mentions.List)
			r.Get("/mentions/counts", mentions.Counts)
			r.Get("/mentions/tier-counts", mentions.TierCounts)
			r.Get("/mentions/{id}", mentions.Get)
			r.Patch("/mentions/{id}/status", mentions.UpdateStatus)

			// Profiles (Pain-Point Monitoring)
			profiles := handler.NewProfileHandler(queries, embedder)
			r.Get("/profiles", profiles.List)
			r.Get("/profiles/{id}", profiles.Get)
			r.Post("/profiles", profiles.Create)
			r.Put("/profiles/{id}", profiles.Update)
			r.Delete("/profiles/{id}", profiles.Delete)

			// AI (intent classification + reply drafting)
			aiHandler := handler.NewAIHandler(queries, cfg.GLMAPIKey, cfg.OpenAIAPIKey)
			r.Post("/mentions/{id}/classify", aiHandler.Classify)
			r.Post("/mentions/{id}/draft-reply", aiHandler.DraftReply)

			// Leads
			leads := handler.NewLeadHandler(queries)
			r.Get("/leads", leads.List)
			r.Get("/leads/counts", leads.Counts)
			r.Get("/leads/{id}", leads.Get)
			r.Post("/leads", leads.Create)
			r.Patch("/leads/{id}/stage", leads.UpdateStage)

			// Keywords
			keywords := handler.NewKeywordHandler(queries)
			r.Get("/keywords", keywords.List)
			r.Get("/keywords/{id}", keywords.Get)
			r.Post("/keywords", keywords.Create)
			r.Put("/keywords/{id}", keywords.Update)
			r.Delete("/keywords/{id}", keywords.Delete)

			// Replies
			replies := handler.NewReplyHandler(queries)
			r.Get("/mentions/{mentionId}/replies", replies.ListByMention)
			r.Post("/replies", replies.Create)
			r.Patch("/replies/{id}/content", replies.UpdateContent)
			r.Patch("/replies/{id}/status", replies.UpdateStatus)

			// Documents (Knowledge Base)
			docs := handler.NewDocumentHandler(queries)
			r.Get("/documents", docs.List)
			r.Get("/documents/{id}", docs.Get)
			r.Post("/documents", docs.Create)
			r.Put("/documents/{id}", docs.Update)
			r.Delete("/documents/{id}", docs.Delete)

			// Analytics
			analytics := handler.NewAnalyticsHandler(queries)
			r.Get("/analytics/overview", analytics.Overview)
			r.Get("/analytics/mentions-per-day", analytics.MentionsPerDay)
			r.Get("/analytics/mentions-per-platform", analytics.MentionsPerPlatform)
			r.Get("/analytics/mentions-per-intent", analytics.MentionsPerIntent)
			r.Get("/analytics/conversion-funnel", analytics.ConversionFunnel)
			r.Get("/analytics/top-keywords", analytics.TopKeywords)

			// Notifications (Slack/Discord webhooks)
			notifs := handler.NewNotificationHandler(queries, cfg.ResendAPIKey)
			r.Get("/notifications/webhooks", notifs.GetWebhookConfig)
			r.Put("/notifications/webhooks", notifs.SaveWebhookConfig)
			r.Post("/notifications/webhooks/test", notifs.TestWebhook)

			// Settings (BYOK API keys)
			settings := handler.NewSettingsHandler(queries, encKey)
			r.Get("/settings/api-keys", settings.GetAPIKeys)
			r.Put("/settings/api-keys", settings.SaveAPIKey)
			r.Delete("/settings/api-keys", settings.DeleteAPIKey)

			// Browser sessions (Pinchtab)
			sessions := handler.NewSessionHandler(queries, encKey, pinchtab)
			r.Get("/settings/sessions", sessions.List)
			r.Put("/settings/sessions/{platform}", sessions.Save)
			r.Delete("/settings/sessions/{platform}", sessions.Delete)
			r.Post("/settings/sessions/{platform}/test", sessions.Test)

			// Chrome extension token management
			r.Get("/settings/extension-token", ext.GetToken)
			r.Post("/settings/extension-token", ext.RotateToken)
			r.Delete("/settings/extension-token", ext.RevokeToken)

			// Onboarding wizard
			onboarding := handler.NewOnboardingHandler(queries)
			r.Get("/settings/onboarding", onboarding.GetOnboardingStatus)
			r.Patch("/settings/onboarding", onboarding.UpdateOnboarding)

			// UTM tracking links
			utm := handler.NewUTMHandler(queries)
			r.Get("/utm-links", utm.List)
			r.Post("/utm-links", utm.Create)
			r.Delete("/utm-links/{id}", utm.Delete)
		})

		// Extension signal ingestion — separate auth (X-Extension-Key) + permissive CORS
		// so MV3 service workers (chrome-extension:// origin) are accepted.
		r.Group(func(r chi.Router) {
			r.Use(cors.Handler(cors.Options{
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST", "PATCH", "OPTIONS"},
				AllowedHeaders: []string{"Content-Type", "X-Extension-Key"},
			}))
			r.Use(middleware.ExtensionKeyAuth(queries))
			r.Post("/extension/signals", ext.IngestSignals)
			r.Get("/extension/mentions", ext.ListMentions)
			r.Get("/extension/reply-queue", ext.GetReplyQueue)
			r.Patch("/extension/replies/{id}/mark-posted", ext.MarkReplyPosted)
		})
	})

	return r
}
