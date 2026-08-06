// Package eventstore handles persistence of inbound webhook events.
package eventstore

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const pgUniqueViolation = "23505"

// ErrDuplicateEvent indicates the event's idempotency key already exists —
// this is not an error condition for the caller, just a signal to
// acknowledge without reprocessing.
var ErrDuplicateEvent = errors.New("duplicate event")

// Insert persists a raw event. It returns ErrDuplicateEvent if an event
// with the same idempotency key already exists, rather than a generic
// database error, so callers can distinguish "already seen" from "actually broken."
func Insert(db *sql.DB, idempotencyKey, source string, payload []byte) error {
	_, err := db.Exec(
		`INSERT INTO events (idempotency_key, source, payload) VALUES ($1, $2, $3)`,
		idempotencyKey, source, payload,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return ErrDuplicateEvent
		}
		return err
	}
	return nil
}
