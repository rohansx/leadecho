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

	"leadecho/internal/api"
	"leadecho/internal/browser"
	"leadecho/internal/config"
	"leadecho/internal/crypto"
	"leadecho/internal/database"
	"leadecho/internal/events/consumers"
	"leadecho/internal/events/publishers"
	streamredis "leadecho/internal/events/redis"
	"leadecho/internal/llm"
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

	// Encryption key for stored BYOK keys and browser session cookies.
	encKey := crypto.DeriveKey(cfg.EncryptionKeyOrDefault())

	llmRouter := llm.NewRouter(queries, encKey, llm.SystemKeys{
		NVIDIAAPIKey:   cfg.NVIDIAAPIKey,
		NVIDIAModel:    cfg.NVIDIAModel,
		DeepSeekAPIKey: cfg.DeepSeekAPIKey,
		GLMAPIKey:      cfg.GLMAPIKey,
		OpenAIAPIKey:   cfg.OpenAIAPIKey,
		VoyageAPIKey:   cfg.VoyageAPIKey,
	}, logger)
	logger.Info().Msg("LLM router initialized")

	streamClient := streamredis.NewClient(redis, logger)
	eventPublisher := publishers.New(queries, streamClient, logger)

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

	// Scrapling stealth fallback sidecar (optional)
	var scrapling *browser.ScraplingClient
	if cfg.ScraplingURL != "" {
		scrapling = browser.NewScrapling(cfg.ScraplingURL, cfg.ScraplingToken)
		logger.Info().Str("url", cfg.ScraplingURL).Msg("Scrapling browser client initialized")
	}

	mon := monitor.New(
		queries,
		logger,
		cfg.ResendAPIKey,
		llmRouter,
		pinchtab,
		camoufox,
		scrapling,
		encKey,
		cfg.ExaAPIKey,
		eventPublisher,
		cfg.StreamsEnabled,
		cfg.StreamsDualWriteEnabled,
		cfg.StreamsInlineFallbackEnabled,
	)
	go mon.Run(ctx, 5*time.Minute)

	if cfg.StreamsEnabled {
		workers := consumers.NewMentionWorkers(
			queries,
			streamClient,
			mon,
			logger,
			cfg.StreamsConsumerName,
			cfg.StreamsBatchSize,
			cfg.StreamsBlockMS,
			cfg.StreamsClaimIdleMS,
			cfg.StreamsMaxAttempts,
		)
		if cfg.StreamsScorerConsumerEnabled {
			go func() {
				if err := workers.StartScorer(ctx); err != nil && err != context.Canceled {
					logger.Error().Err(err).Msg("mention scorer worker stopped")
				}
			}()
		}
		if cfg.StreamsNotifierConsumerEnabled {
			go func() {
				if err := workers.StartNotifier(ctx); err != nil && err != context.Canceled {
					logger.Error().Err(err).Msg("mention notifier worker stopped")
				}
			}()
		}
		if cfg.StreamsRetryConsumerEnabled {
			go func() {
				if err := workers.StartRetryManager(ctx); err != nil && err != context.Canceled {
					logger.Error().Err(err).Msg("streams retry manager stopped")
				}
			}()
		}
	}

	// Build router
	router := api.NewRouter(logger, db, redis, cfg, llmRouter, eventPublisher, pinchtab, scrapling, mon)

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
