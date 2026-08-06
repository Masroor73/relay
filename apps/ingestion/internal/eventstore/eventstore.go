// Package eventstore handles persistence of inbound webhook events via the
// transactional outbox pattern: the event and its outbox entry are written
// atomically, so an event can never be recorded without also being queued
// for processing (see ARCHITECTURE.md §4.2).
package eventstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"
)

const pgUniqueViolation = "23505"

// ErrDuplicateEvent indicates the event's idempotency key already exists —
// this is not an error condition for the caller, just a signal to
// acknowledge without reprocessing.
var ErrDuplicateEvent = errors.New("duplicate event")

// Insert persists a raw event and its corresponding outbox entry
// atomically. It returns ErrDuplicateEvent if an event with the same
// idempotency key already exists, rather than a generic database error.
func Insert(ctx context.Context, db *sql.DB, idempotencyKey, source string, payload []byte) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Error("failed to rollback transaction", "error", err)
		}
	}()

	var eventID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO events (idempotency_key, source, payload) VALUES ($1, $2, $3) RETURNING event_id`,
		idempotencyKey, source, payload,
	).Scan(&eventID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return ErrDuplicateEvent
		}
		return fmt.Errorf("insert event: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO outbox (event_id) VALUES ($1)`,
		eventID,
	)
	if err != nil {
		return fmt.Errorf("insert outbox entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
