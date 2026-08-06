// Command seed inserts sample events and orders for manual testing and
// dashboard development, ahead of the processor (Milestone 3) existing to
// produce real orders. Safe to run repeatedly — duplicate events are
// skipped via the same idempotency check used in production.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Masroor73/relay/apps/ingestion/internal/eventstore"
	"github.com/Masroor73/relay/apps/ingestion/internal/logging"
)

type sampleEvent struct {
	idempotencyKey string
	amountCents    int64
	status         string
}

var sampleEvents = []sampleEvent{
	{idempotencyKey: "evt_seed_001", amountCents: 4999, status: "confirmed"},
	{idempotencyKey: "evt_seed_002", amountCents: 12500, status: "confirmed"},
	{idempotencyKey: "evt_seed_003", amountCents: 799, status: "confirmed"},
}

func main() {
	slog.SetDefault(logging.New())

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		slog.Error("failed to open database connection", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx := context.Background()
	inserted, skipped := 0, 0

	for _, se := range sampleEvents {
		payload := fmt.Appendf(nil,
			`{"id":"%s","api_version":"2025-02-24.acacia","type":"payment_intent.succeeded","amount":%d}`,
			se.idempotencyKey, se.amountCents,
		)

		err := eventstore.Insert(ctx, db, se.idempotencyKey, "stripe", payload)
		switch {
		case err == nil:
			if seedErr := insertSampleOrder(ctx, db, se); seedErr != nil {
				slog.Error("failed to insert sample order", "idempotency_key", se.idempotencyKey, "error", seedErr)
				continue
			}
			slog.Info("seeded event and order", "idempotency_key", se.idempotencyKey)
			inserted++
		case errors.Is(err, eventstore.ErrDuplicateEvent):
			slog.Info("event already seeded, skipping", "idempotency_key", se.idempotencyKey)
			skipped++
		default:
			slog.Error("failed to seed event", "idempotency_key", se.idempotencyKey, "error", err)
			os.Exit(1)
		}
	}

	slog.Info("seed complete", "inserted", inserted, "skipped", skipped)
}

func insertSampleOrder(ctx context.Context, db *sql.DB, se sampleEvent) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO orders (stripe_event_id, amount_cents, status)
		 SELECT event_id, $1, $2 FROM events WHERE idempotency_key = $3
		 ON CONFLICT DO NOTHING`,
		se.amountCents, se.status, se.idempotencyKey,
	)
	return err
}
