// Command server starts the Relay ingestion HTTP service.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Masroor73/relay/apps/ingestion/internal/config"
	"github.com/Masroor73/relay/apps/ingestion/internal/db"
	"github.com/Masroor73/relay/apps/ingestion/internal/logging"
	"github.com/Masroor73/relay/apps/ingestion/internal/server"
)

// shutdownGracePeriod bounds how long Shutdown waits for in-flight
// requests to finish before forcibly closing remaining connections.
const shutdownGracePeriod = 25 * time.Second

func main() {
	slog.SetDefault(logging.New())

	cfg, err := config.Load()
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

	srv := server.New(cfg, conn)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("ingestion service starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutdown signal received, draining in-flight requests")

	// Shutdown's own context bounds how long it waits for in-flight
	// requests — deliberately a fresh timeout, not ctx (already Done()
	// by the time we get here, since that's what triggered this branch;
	// reusing it would give Shutdown zero time to drain anything).
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGracePeriod)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("shutdown grace period exceeded, forcing exit", "error", err)
		return
	}

	slog.Info("server shut down cleanly")
}
