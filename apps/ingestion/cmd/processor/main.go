// Command processor polls the outbox for pending events and executes their
// downstream effects, with retry/backoff and dead-letter promotion on
// exhaustion.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Masroor73/relay/apps/ingestion/internal/config"
	"github.com/Masroor73/relay/apps/ingestion/internal/db"
	"github.com/Masroor73/relay/apps/ingestion/internal/logging"
	"github.com/Masroor73/relay/apps/ingestion/internal/outbox"
	"github.com/Masroor73/relay/apps/ingestion/internal/processor"
)

// shutdownGracePeriod bounds how long the process waits for an in-flight
// poll cycle to finish after a shutdown signal, before forcing exit — a
// safety net in case a downstream effect hangs (e.g. an unresponsive DB).
const shutdownGracePeriod = 25 * time.Second

func main() {
	slog.SetDefault(logging.New())

	cfg, err := config.LoadProcessor()
	if err != nil {
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	conn, err := db.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to open database connection", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		outbox.Poll(ctx, conn, processor.HandlePaymentIntentSucceeded)
	}()

	slog.Info("processor service starting")

	<-ctx.Done()
	slog.Info("shutdown signal received, draining in-flight work")

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("processor shut down cleanly")
	case <-time.After(shutdownGracePeriod):
		slog.Warn("shutdown grace period exceeded, forcing exit")
	}
}
