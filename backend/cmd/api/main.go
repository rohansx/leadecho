package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"leadecho/internal/ai"
	"leadecho/internal/api"
	"leadecho/internal/browser"
	"leadecho/internal/config"
	"leadecho/internal/crypto"
	"leadecho/internal/database"
	"leadecho/internal/embedding"
	"leadecho/internal/monitor"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Logger
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	if os.Getenv("ENVIRONMENT") == "development" {
		logger = logger.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	// Load config
	cfg, err := config.Load(ctx)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load config")
	}

	// Connect to PostgreSQL
	db, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to postgres")
	}
	defer db.Close()
	logger.Info().Msg("connected to PostgreSQL")

	// Connect to Redis
	redis, err := database.NewRedisClient(ctx, cfg.RedisURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer redis.Close()
	logger.Info().Msg("connected to Redis")

	// Start social monitoring worker (polls Reddit, HN every 5 minutes)
	queries := database.New(db)

	// Embedding client (optional — nil if no Voyage API key)
	var embedder *embedding.Client
	if cfg.VoyageAPIKey != "" {
		embedder = embedding.New(cfg.VoyageAPIKey)
		logger.Info().Msg("Voyage AI embedding client initialized")
	}

	// AI provider (optional — nil if no LLM key)
	var aiProvider *ai.Provider
	if cfg.GLMAPIKey != "" {
		aiProvider = &ai.Provider{
			Name:    "glm",
			APIKey:  cfg.GLMAPIKey,
			BaseURL: "https://open.bigmodel.cn/api/paas/v4",
			Model:   "glm-4.5-flash",
		}
	} else if cfg.OpenAIAPIKey != "" {
		aiProvider = &ai.Provider{
			Name:    "openai",
			APIKey:  cfg.OpenAIAPIKey,
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4o-mini",
		}
	}

	// Encryption key for session cookies
	encKey := crypto.DeriveKey(cfg.EncryptionKeyOrDefault())

	// Pinchtab browser sidecar (optional)
	var pinchtab *browser.PinchtabClient
	if cfg.PinchtabToken != "" {
		pinchtab = browser.New(cfg.PinchtabURL, cfg.PinchtabToken)
		logger.Info().Str("url", cfg.PinchtabURL).Msg("Pinchtab browser client initialized")
	}

	// Camoufox Pro-tier stealth Firefox sidecar (optional)
	var camoufox *browser.CamoufoxClient
	if cfg.CamoufoxURL != "" {
		camoufox = browser.NewCamoufox(cfg.CamoufoxURL, cfg.CamoufoxToken)
		logger.Info().Str("url", cfg.CamoufoxURL).Msg("Camoufox browser client initialized")
	}

	mon := monitor.New(queries, logger, cfg.ResendAPIKey, embedder, aiProvider, pinchtab, camoufox, encKey)
	go mon.Run(ctx, 5*time.Minute)

	// Build router
	router := api.NewRouter(logger, db, redis, cfg, embedder, pinchtab, mon)

	// Start server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		logger.Info().Msg("shutting down server...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("server shutdown error")
		}
		cancel()
	}()

	logger.Info().Int("port", cfg.Port).Msg("starting LeadEcho API server")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal().Err(err).Msg("server error")
	}
	logger.Info().Msg("server stopped gracefully")
}
