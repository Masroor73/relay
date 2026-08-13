package outbox

import (
	"context"
	"database/sql"
	"fmt"
)

// promoteToDeadLetter moves a row that has exhausted MaxAttempts into
// dead_letter_events with its final error attached, and removes it from
// outbox — ending the active retry cycle per ARCHITECTURE.md §4.4.
func promoteToDeadLetter(ctx context.Context, db *sql.DB, row OutboxRow, finalError string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO dead_letter_events (event_id, final_error) VALUES ($1, $2)`,
		row.EventID, finalError,
	)
	if err != nil {
		return fmt.Errorf("insert dead letter event: %w", err)
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM outbox WHERE outbox_id = $1`, row.OutboxID)
	if err != nil {
		return fmt.Errorf("delete outbox row: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
