// Package processor implements the downstream effects executed for
// successfully claimed outbox rows — the business logic layer on top of
// internal/outbox's polling mechanics.
package processor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Masroor73/relay/apps/ingestion/internal/outbox"
)

// eventPayload mirrors the simplified, flat event JSON shape used
// throughout this project's tests and cmd/seed — not Stripe's real
// nested event schema (which nests amount under data.object.amount). A
// production integration would parse Stripe's actual structure; this
// project's payloads are intentionally simplified for demonstration,
// documented here rather than silently assumed.
type eventPayload struct {
	Type   string `json:"type"`
	Amount int64  `json:"amount"`
}

// HandlePaymentIntentSucceeded is the outbox.Handler that executes the
// downstream effect for a claimed row: writing the corresponding orders
// row. Per ARCHITECTURE.md §6, only payment_intent.succeeded events
// currently produce an order — other event types are acknowledged as
// processed with no further effect, rather than treated as an error.
func HandlePaymentIntentSucceeded(ctx context.Context, tx *sql.Tx, row outbox.OutboxRow) error {
	var payloadBytes []byte
	if err := tx.QueryRowContext(ctx,
		`SELECT payload FROM events WHERE event_id = $1`, row.EventID,
	).Scan(&payloadBytes); err != nil {
		return fmt.Errorf("fetch event payload: %w", err)
	}

	var payload eventPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return fmt.Errorf("parse event payload: %w", err)
	}

	if payload.Type != "payment_intent.succeeded" {
		return nil
	}

	_, err := tx.ExecContext(ctx,
		`INSERT INTO orders (stripe_event_id, amount_cents, status)
		 VALUES ($1, $2, 'confirmed')
		 ON CONFLICT (stripe_event_id) DO NOTHING`,
		row.EventID, payload.Amount,
	)
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
	}

	return nil
}
