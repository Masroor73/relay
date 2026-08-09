package outbox

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestProcessOnce_ConcurrentPollersDoNotDoubleProcess proves the actual
// point of this issue: two pollers racing for the same row must result in
// exactly one handler invocation, not zero and not two.
func TestProcessOnce_ConcurrentPollersDoNotDoubleProcess(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test that requires a real Postgres connection")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("failed to open database connection: %v", err)
	}
	defer db.Close()

	const idempotencyKey = "evt_outbox_poll_test"

	cleanup := func() {
		if _, err := db.Exec(`DELETE FROM outbox WHERE event_id IN (SELECT event_id FROM events WHERE idempotency_key = $1)`, idempotencyKey); err != nil {
			t.Logf("cleanup: failed to delete outbox rows: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM events WHERE idempotency_key = $1`, idempotencyKey); err != nil {
			t.Logf("cleanup: failed to delete events row: %v", err)
		}
	}
	cleanup()
	defer cleanup()

	// This test asserts ProcessOnce claims exactly one row across two
	// concurrent callers — that assertion only holds if this test's row is
	// the *only* due row in the table. Earlier manual testing (e.g. `make
	// seed`, run before a processor existed to drain it) can leave
	// unrelated rows sitting in outbox indefinitely, which would let two
	// concurrent pollers each legitimately claim a different leftover row
	// and produce a false failure that looks like a race bug but isn't.
	// Clearing the whole table here is safe: outbox has no child rows
	// depending on it (unlike events, which orders references via FK).
	if _, err := db.Exec(`DELETE FROM outbox`); err != nil {
		t.Fatalf("failed to clear outbox table for test isolation: %v", err)
	}

	var eventID string
	err = db.QueryRow(
		`INSERT INTO events (idempotency_key, source, payload) VALUES ($1, $2, $3) RETURNING event_id`,
		idempotencyKey, "stripe", []byte(`{"id":"evt_outbox_poll_test"}`),
	).Scan(&eventID)
	if err != nil {
		t.Fatalf("failed to insert test event: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO outbox (event_id) VALUES ($1)`, eventID); err != nil {
		t.Fatalf("failed to insert test outbox row: %v", err)
	}

	var handlerCalls int32
	handler := func(ctx context.Context, tx *sql.Tx, row OutboxRow) error {
		atomic.AddInt32(&handlerCalls, 1)
		time.Sleep(50 * time.Millisecond) // widen the race window deliberately
		return nil
	}

	var wg sync.WaitGroup
	ready := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			ProcessOnce(context.Background(), db, handler)
		}()
	}
	close(ready)
	wg.Wait()

	if got := atomic.LoadInt32(&handlerCalls); got != 1 {
		t.Errorf("expected handler to be called exactly once, got %d", got)
	}

	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM outbox WHERE event_id = $1`, eventID).Scan(&remaining); err != nil {
		t.Fatalf("failed to count remaining outbox rows: %v", err)
	}
	if remaining != 0 {
		t.Errorf("expected outbox row to be deleted after processing, got %d remaining", remaining)
	}
}
