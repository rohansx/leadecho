package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"leadecho/internal/api"
	"leadecho/internal/metrics"
	"leadecho/internal/platform"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := platform.NewLogger()

	shared, err := platform.Bootstrap(ctx, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("bootstrap failed")
	}
	defer shared.Close()

	if shared.Config.IsWorker() {
		logger.Fatal().Msg("PROCESS_ROLE=worker is not valid for cmd/api; use cmd/worker")
	}

	shared.StartMonitor(ctx, 5*time.Minute)
	shared.StartStreamWorkers(ctx)
	shared.StartMetricsCollector(ctx)

	router := api.NewRouter(
		logger,
		shared.DB,
		shared.Redis,
		shared.Config,
		shared.LLMRouter,
		shared.EventPublisher,
		shared.Pinchtab,
		shared.Scrapling,
		shared.Monitor,
		shared.ReplyDrafter,
	)
	if shared.Config.MetricsEnabled {
		router.Handle("/metrics", metrics.Handler())
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", shared.Config.Port),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		platform.WaitForSignal(cancel)
		logger.Info().Msg("shutting down API server...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("server shutdown error")
		}
	}()

	logger.Info().Int("port", shared.Config.Port).Msg("starting LeadEcho API server")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal().Err(err).Msg("server error")
	}
	logger.Info().Msg("API server stopped gracefully")
}
