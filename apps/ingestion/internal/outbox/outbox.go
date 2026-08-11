// Package outbox implements the processor's polling loop, claiming pending
// rows safely under concurrent access via a held row lock, and delegating
// each claimed row's downstream effect to a handler function running
// inside the same transaction that holds the lock.
package outbox

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"
)

const pollInterval = 2 * time.Second

// MaxAttempts is the number of processing attempts allowed before a row is
// promoted to the dead-letter table (ENG-28) rather than retried again.
const MaxAttempts = 6

// OutboxRow represents a single claimed row pending processing.
type OutboxRow struct {
	OutboxID     string
	EventID      string
	AttemptCount int
}

// Handler executes the downstream effect for a claimed row, within the
// same transaction that holds the row's lock — so the effect (e.g.
// writing to orders) and the outbox row's removal commit or roll back
// together atomically. Returning an error rolls back the transaction,
// leaving the row in place for a later retry attempt.
type Handler func(ctx context.Context, tx *sql.Tx, row OutboxRow) error

// Poll runs the polling loop until ctx is cancelled, claiming and
// processing due outbox rows on a fixed interval.
func Poll(ctx context.Context, db *sql.DB, handle Handler) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("outbox polling stopped")
			return
		case <-ticker.C:
			ProcessOnce(ctx, db, handle)
		}
	}
}

// ProcessOnce claims and processes at most one due row, if any exist. It
// is exported separately from Poll so tests can exercise a single
// claim/process cycle directly without waiting on the ticker.
func ProcessOnce(ctx context.Context, db *sql.DB, handle Handler) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		return
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Error("failed to rollback transaction", "error", err)
		}
	}()

	row, ok, err := claimNextRow(ctx, tx)
	if err != nil {
		slog.Error("failed to claim outbox row", "error", err)
		return
	}
	if !ok {
		return // nothing due right now — normal, not an error; rollback is a no-op
	}

	if handleErr := handle(ctx, tx, row); handleErr != nil {
		slog.Warn("failed to process outbox row, scheduling retry", "event_id", row.EventID, "attempt", row.AttemptCount+1, "error", handleErr)

		// Roll back explicitly here, before scheduling the retry — the
		// retry update runs on a separate pooled connection and would
		// otherwise deadlock waiting for this transaction's row lock to
		// release, which only happens once this function returns (via the
		// deferred rollback above). Explicit rollback here releases the
		// lock immediately instead.
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			slog.Error("failed to rollback transaction", "error", err)
		}

		if err := scheduleRetry(ctx, db, row); err != nil {
			slog.Error("failed to schedule retry", "event_id", row.EventID, "error", err)
		}
		return
	}

	if err := deleteRow(ctx, tx, row.OutboxID); err != nil {
		slog.Error("failed to delete processed outbox row", "event_id", row.EventID, "error", err)
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "event_id", row.EventID, "error", err)
		return
	}

	slog.Info("outbox row processed", "event_id", row.EventID)
}

// claimNextRow locks and returns one due row within tx, skipping any
// already locked by a concurrent poller's transaction. The lock is held
// for the lifetime of tx — covering the handler's effect execution and
// the row's deletion, not just this initial SELECT.
func claimNextRow(ctx context.Context, tx *sql.Tx) (OutboxRow, bool, error) {
	var row OutboxRow
	err := tx.QueryRowContext(ctx,
		`SELECT outbox_id, event_id, attempt_count FROM outbox
		 WHERE next_attempt_at <= now()
		 ORDER BY next_attempt_at
		 FOR UPDATE SKIP LOCKED
		 LIMIT 1`,
	).Scan(&row.OutboxID, &row.EventID, &row.AttemptCount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OutboxRow{}, false, nil
		}
		return OutboxRow{}, false, err
	}
	return row, true, nil
}

// scheduleRetry increments the row's attempt count and sets next_attempt_at
// per the backoff schedule. Run in its own transaction, separate from the
// (already rolled-back) processing transaction — the failed effect must
// not persist, but the retry bookkeeping must.
func scheduleRetry(ctx context.Context, db *sql.DB, row OutboxRow) error {
	newAttemptCount := row.AttemptCount + 1
	delay := nextBackoff(newAttemptCount)

	_, err := db.ExecContext(ctx,
		`UPDATE outbox
		 SET attempt_count = $1, next_attempt_at = now() + $2::interval
		 WHERE outbox_id = $3`,
		newAttemptCount, delay.String(), row.OutboxID,
	)
	return err
}

func deleteRow(ctx context.Context, tx *sql.Tx, outboxID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM outbox WHERE outbox_id = $1`, outboxID)
	return err
}
