package platform

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"leadecho/internal/api/handler"
	"leadecho/internal/browser"
	"leadecho/internal/config"
	"leadecho/internal/crypto"
	"leadecho/internal/database"
	"leadecho/internal/events/publishers"
	streamredis "leadecho/internal/events/redis"
	"leadecho/internal/knowledge"
	"leadecho/internal/llm"
	"leadecho/internal/metrics"
	"leadecho/internal/monitor"
	"leadecho/internal/reply"
	"leadecho/internal/researcher"
	"leadecho/internal/streamworkers"
	"leadecho/internal/workflow"
)

type Shared struct {
	Config         *config.Config
	Logger         zerolog.Logger
	DB             *pgxpool.Pool
	Redis          *goredis.Client
	Queries        *database.Queries
	LLMRouter      *llm.Router
	StreamClient   *streamredis.Client
	EventPublisher *publishers.Publisher
	Pinchtab       *browser.PinchtabClient
	Camoufox       *browser.CamoufoxClient
	Scrapling      *browser.ScraplingClient
	Knowledge      *knowledge.Service
	Researcher     *researcher.Service
	Monitor        *monitor.Monitor
	ReplyDrafter   *reply.Drafter
	WorkflowEngine *workflow.Engine
}

func Bootstrap(ctx context.Context, logger zerolog.Logger) (*Shared, error) {
	cfg, err := config.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	db, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	redis, err := database.NewRedisClient(ctx, cfg.RedisURL)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	queries := database.New(db)
	encKey := crypto.DeriveKey(cfg.EncryptionKeyOrDefault())

	llmRouter := llm.NewRouter(queries, encKey, llm.SystemKeys{
		NVIDIAAPIKey:   cfg.NVIDIAAPIKey,
		NVIDIAModel:    cfg.NVIDIAModel,
		DeepSeekAPIKey: cfg.DeepSeekAPIKey,
		GLMAPIKey:      cfg.GLMAPIKey,
		OpenAIAPIKey:   cfg.OpenAIAPIKey,
		OllamaAPIKey:   cfg.OllamaAPIKey,
		OllamaModel:    cfg.OllamaModel,
		VoyageAPIKey:   cfg.VoyageAPIKey,
	}, logger)

	streamClient := streamredis.NewClient(redis, logger)
	eventPublisher := publishers.New(queries, streamClient, logger)

	var pinchtab *browser.PinchtabClient
	if cfg.PinchtabToken != "" {
		pinchtab = browser.New(cfg.PinchtabURL, cfg.PinchtabToken)
	}

	var camoufox *browser.CamoufoxClient
	if cfg.CamoufoxURL != "" {
		camoufox = browser.NewCamoufox(cfg.CamoufoxURL, cfg.CamoufoxToken)
	}

	var scrapling *browser.ScraplingClient
	if cfg.ScraplingURL != "" {
		scrapling = browser.NewScrapling(cfg.ScraplingURL, cfg.ScraplingToken)
	}

	knowledgeSvc := knowledge.NewService(queries, llmRouter, logger)
	researcherSvc := researcher.NewService(queries, logger)

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
		cfg.StreamsEnabled && cfg.StreamsQualifierConsumerEnabled,
		researcherSvc,
	)
	mon.SetGoogleNewsEnabled(cfg.GoogleNewsEnabled)

	replyDrafter := reply.NewDrafter(queries, llmRouter, scrapling, knowledgeSvc)
	workflowEngine := workflow.NewEngine(queries, eventPublisher, logger)

	return &Shared{
		Config:         cfg,
		Logger:         logger,
		DB:             db,
		Redis:          redis,
		Queries:        queries,
		LLMRouter:      llmRouter,
		StreamClient:   streamClient,
		EventPublisher: eventPublisher,
		Pinchtab:       pinchtab,
		Camoufox:       camoufox,
		Scrapling:      scrapling,
		Knowledge:      knowledgeSvc,
		Researcher:     researcherSvc,
		Monitor:        mon,
		ReplyDrafter:   replyDrafter,
		WorkflowEngine: workflowEngine,
	}, nil
}

func (s *Shared) Close() {
	if s.Redis != nil {
		s.Redis.Close()
	}
	if s.DB != nil {
		s.DB.Close()
	}
}

func (s *Shared) StartMonitor(ctx context.Context, interval time.Duration) {
	if !s.Config.ShouldRunMonitor() {
		s.Logger.Info().Msg("monitor disabled")
		return
	}
	go s.Monitor.Run(ctx, interval)
}

func (s *Shared) StartStreamWorkers(ctx context.Context) {
	if !s.Config.ShouldRunStreamWorkers() {
		s.Logger.Info().Msg("stream workers disabled for this process role")
		return
	}
	streamworkers.New(streamworkers.Options{
		Config:         s.Config,
		Logger:         s.Logger,
		Queries:        s.Queries,
		StreamClient:   s.StreamClient,
		Monitor:        s.Monitor,
		ReplyDrafter:   s.ReplyDrafter,
		WorkflowEngine: s.WorkflowEngine,
	}).Start(ctx)
}

func (s *Shared) StartMetricsCollector(ctx context.Context) {
	if !s.Config.MetricsEnabled || !s.Config.StreamsEnabled {
		return
	}
	collector := metrics.NewCollector(s.StreamClient, s.Queries, s.Logger, s.Config.MetricsCollectInterval())
	go collector.Run(ctx)
}

func (s *Shared) ServeAuxHTTP(ctx context.Context, addr string) *http.Server {
	mux := http.NewServeMux()
	health := handler.NewHealthHandler(s.DB, s.Redis)
	mux.HandleFunc("/healthz", health.Healthz)
	mux.HandleFunc("/readyz", health.Readyz)
	if s.Config.MetricsEnabled {
		mux.Handle("/metrics", metrics.Handler())
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		s.Logger.Info().Str("addr", addr).Msg("auxiliary HTTP server started")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.Logger.Error().Err(err).Msg("auxiliary HTTP server error")
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	return srv
}

func NewLogger() zerolog.Logger {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	if os.Getenv("ENVIRONMENT") == "development" {
		logger = logger.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
	return logger
}

func WaitForSignal(cancel context.CancelFunc) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	cancel()
}
