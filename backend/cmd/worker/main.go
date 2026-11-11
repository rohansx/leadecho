package main

import (
	"context"
	"fmt"
	"time"

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

	if !shared.Config.IsWorker() {
		logger.Fatal().Msg("set PROCESS_ROLE=worker to run cmd/worker")
	}

	shared.StartMonitor(ctx, 5*time.Minute)
	shared.StartDigests(ctx)     // digests (P1 #13)
	shared.StartDemand(ctx)      // demand themes (daily)
	shared.StartAutomations(ctx) // automations (P2 #17)
	shared.StartStreamWorkers(ctx)
	shared.StartMetricsCollector(ctx)

	addr := fmt.Sprintf(":%d", shared.Config.WorkerHealthPort)
	shared.ServeAuxHTTP(ctx, addr)

	go func() {
		platform.WaitForSignal(cancel)
		logger.Info().Msg("shutting down worker...")
	}()

	logger.Info().
		Str("consumer_name", shared.Config.StreamsConsumerName).
		Int("health_port", shared.Config.WorkerHealthPort).
		Msg("LeadEcho stream worker started")

	<-ctx.Done()
	logger.Info().Msg("worker stopped gracefully")
}
