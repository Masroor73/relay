package outbox

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestProcessOnce_RetriesOnFailureThenSucceeds proves the retry path:
// a handler that fails a fixed number of times before succeeding results
// in exactly that many attempts, ending with the row deleted from outbox
// and no dead-letter entry. Forces next_attempt_at into the past between
// calls rather than waiting through real backoff windows — the backoff
// durations themselves are already unit-tested in backoff_test.go; this
// test's job is the integration behavior, not the timing math.
func TestProcessOnce_RetriesOnFailureThenSucceeds(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test that requires a real Postgres connection")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("failed to open database connection: %v", err)
	}
	defer db.Close()

	const idempotencyKey = "evt_retry_success_test"

	cleanup := func() {
		if _, err := db.Exec(`DELETE FROM dead_letter_events WHERE event_id IN (SELECT event_id FROM events WHERE idempotency_key = $1)`, idempotencyKey); err != nil {
			t.Logf("cleanup: failed to delete dead_letter_events rows: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM outbox WHERE event_id IN (SELECT event_id FROM events WHERE idempotency_key = $1)`, idempotencyKey); err != nil {
			t.Logf("cleanup: failed to delete outbox rows: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM events WHERE idempotency_key = $1`, idempotencyKey); err != nil {
			t.Logf("cleanup: failed to delete events row: %v", err)
		}
	}
	cleanup()
	defer cleanup()

	// Clear the whole outbox table for isolation — leftover due rows from
	// other tests or manual testing let ProcessOnce claim an unrelated
	// row instead of this test's, producing a false result. Learned the
	// hard way in ENG-25's concurrency test.
	if _, err := db.Exec(`DELETE FROM outbox`); err != nil {
		t.Fatalf("failed to clear outbox table for test isolation: %v", err)
	}

	var eventID string
	err = db.QueryRow(
		`INSERT INTO events (idempotency_key, source, payload) VALUES ($1, $2, $3) RETURNING event_id`,
		idempotencyKey, "stripe", []byte(`{"id":"evt_retry_success_test"}`),
	).Scan(&eventID)
	if err != nil {
		t.Fatalf("failed to insert test event: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO outbox (event_id) VALUES ($1)`, eventID); err != nil {
		t.Fatalf("failed to insert test outbox row: %v", err)
	}

	const failuresBeforeSuccess = 2
	callCount := 0
	handler := func(ctx context.Context, tx *sql.Tx, row OutboxRow) error {
		callCount++
		if callCount <= failuresBeforeSuccess {
			return errors.New("forced failure for retry test")
		}
		return nil
	}

	for i := 0; i < failuresBeforeSuccess+1; i++ {
		ProcessOnce(context.Background(), db, handler)
		if _, err := db.Exec(`UPDATE outbox SET next_attempt_at = now() WHERE event_id = $1`, eventID); err != nil {
			t.Logf("failed to force next_attempt_at forward (row may already be deleted after success): %v", err)
		}
	}

	if callCount != failuresBeforeSuccess+1 {
		t.Errorf("expected handler to be called %d times, got %d", failuresBeforeSuccess+1, callCount)
	}

	var remainingOutbox int
	if err := db.QueryRow(`SELECT COUNT(*) FROM outbox WHERE event_id = $1`, eventID).Scan(&remainingOutbox); err != nil {
		t.Fatalf("failed to count remaining outbox rows: %v", err)
	}
	if remainingOutbox != 0 {
		t.Errorf("expected outbox row to be deleted after eventual success, got %d remaining", remainingOutbox)
	}

	var deadLetterCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dead_letter_events WHERE event_id = $1`, eventID).Scan(&deadLetterCount); err != nil {
		t.Fatalf("failed to count dead_letter_events rows: %v", err)
	}
	if deadLetterCount != 0 {
		t.Errorf("expected no dead-letter entry for an eventually-successful event, got %d", deadLetterCount)
	}
}

// TestProcessOnce_ExceedsMaxAttempts_PromotesToDeadLetter proves the
// dead-letter path: a handler that always fails results in the row being
// promoted to dead_letter_events with the final error attached, and
// removed from outbox, after exactly MaxAttempts attempts.
func TestProcessOnce_ExceedsMaxAttempts_PromotesToDeadLetter(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test that requires a real Postgres connection")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("failed to open database connection: %v", err)
	}
	defer db.Close()

	const idempotencyKey = "evt_retry_dlq_test"

	cleanup := func() {
		if _, err := db.Exec(`DELETE FROM dead_letter_events WHERE event_id IN (SELECT event_id FROM events WHERE idempotency_key = $1)`, idempotencyKey); err != nil {
			t.Logf("cleanup: failed to delete dead_letter_events rows: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM outbox WHERE event_id IN (SELECT event_id FROM events WHERE idempotency_key = $1)`, idempotencyKey); err != nil {
			t.Logf("cleanup: failed to delete outbox rows: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM events WHERE idempotency_key = $1`, idempotencyKey); err != nil {
			t.Logf("cleanup: failed to delete events row: %v", err)
		}
	}
	cleanup()
	defer cleanup()

	if _, err := db.Exec(`DELETE FROM outbox`); err != nil {
		t.Fatalf("failed to clear outbox table for test isolation: %v", err)
	}

	var eventID string
	err = db.QueryRow(
		`INSERT INTO events (idempotency_key, source, payload) VALUES ($1, $2, $3) RETURNING event_id`,
		idempotencyKey, "stripe", []byte(`{"id":"evt_retry_dlq_test"}`),
	).Scan(&eventID)
	if err != nil {
		t.Fatalf("failed to insert test event: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO outbox (event_id) VALUES ($1)`, eventID); err != nil {
		t.Fatalf("failed to insert test outbox row: %v", err)
	}

	const finalErrorMessage = "forced permanent failure for dead-letter test"
	handler := func(ctx context.Context, tx *sql.Tx, row OutboxRow) error {
		return errors.New(finalErrorMessage)
	}

	for i := 0; i < MaxAttempts; i++ {
		ProcessOnce(context.Background(), db, handler)
		if _, err := db.Exec(`UPDATE outbox SET next_attempt_at = now() WHERE event_id = $1`, eventID); err != nil {
			t.Logf("failed to force next_attempt_at forward (row may already be dead-lettered): %v", err)
		}
	}

	var remainingOutbox int
	if err := db.QueryRow(`SELECT COUNT(*) FROM outbox WHERE event_id = $1`, eventID).Scan(&remainingOutbox); err != nil {
		t.Fatalf("failed to count remaining outbox rows: %v", err)
	}
	if remainingOutbox != 0 {
		t.Errorf("expected outbox row to be removed after exceeding MaxAttempts, got %d remaining", remainingOutbox)
	}

	var actualError string
	err = db.QueryRow(`SELECT final_error FROM dead_letter_events WHERE event_id = $1`, eventID).Scan(&actualError)
	if err != nil {
		t.Fatalf("expected a dead_letter_events row to exist, query failed: %v", err)
	}
	if actualError != finalErrorMessage {
		t.Errorf("expected final_error %q, got %q", finalErrorMessage, actualError)
	}
}
