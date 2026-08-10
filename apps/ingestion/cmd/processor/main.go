// Command processor polls the outbox for pending events and executes their
// downstream effects, with retry/backoff and dead-letter promotion on
// exhaustion.
package main

import (
	"context"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Masroor73/relay/apps/ingestion/internal/config"
	"github.com/Masroor73/relay/apps/ingestion/internal/db"
	"github.com/Masroor73/relay/apps/ingestion/internal/logging"
	"github.com/Masroor73/relay/apps/ingestion/internal/outbox"
	"github.com/Masroor73/relay/apps/ingestion/internal/processor"
)

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

	slog.Info("processor service starting")
	outbox.Poll(context.Background(), conn, processor.HandlePaymentIntentSucceeded)
}
