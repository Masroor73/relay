// Command processor polls the outbox for pending events and executes their
// downstream effects, with retry/backoff and dead-letter promotion on
// exhaustion.
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Masroor73/relay/apps/ingestion/internal/config"
	"github.com/Masroor73/relay/apps/ingestion/internal/db"
	"github.com/Masroor73/relay/apps/ingestion/internal/logging"
	"github.com/Masroor73/relay/apps/ingestion/internal/outbox"
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

	// Placeholder handler — real downstream effect execution (writing to
	// orders, per ARCHITECTURE.md §6) lands in ENG-26.
	handler := func(ctx context.Context, tx *sql.Tx, row outbox.OutboxRow) error {
		slog.Info("processing outbox row (placeholder handler)", "event_id", row.EventID)
		return nil
	}

	outbox.Poll(context.Background(), conn, handler)
}
