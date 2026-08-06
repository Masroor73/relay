package eventstore

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestInsert_ConcurrentDuplicateEvents is the milestone's core proof: it
// fires two identical inserts at the exact same instant and confirms the
// database's UNIQUE constraint on idempotency_key — not application logic —
// is what enforces exactly-once persistence under a real race.
func TestInsert_ConcurrentDuplicateEvents(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test that requires a real Postgres connection")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("failed to open database connection: %v", err)
	}
	defer db.Close()

	const idempotencyKey = "evt_concurrent_test"
	const source = "stripe"
	payload := []byte(`{"id":"evt_concurrent_test","type":"payment_intent.succeeded"}`)

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

	const numConcurrent = 2
	var wg sync.WaitGroup
	errs := make([]error, numConcurrent)

	// A closed channel acts as a broadcast signal: every goroutine blocks on
	// <-ready until it's closed, at which point all of them unblock at once.
	// This is what makes the delivery genuinely simultaneous rather than
	// merely "issued close together" — a sequential test would pass even
	// with a broken check-then-insert implementation, which is exactly the
	// failure mode this test exists to catch.
	ready := make(chan struct{})

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-ready
			errs[i] = Insert(context.Background(), db, idempotencyKey, source, payload)
		}(i)
	}
	close(ready)
	wg.Wait()

	successCount, duplicateCount := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, ErrDuplicateEvent):
			duplicateCount++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if successCount != 1 {
		t.Errorf("expected exactly 1 successful insert, got %d", successCount)
	}
	if duplicateCount != 1 {
		t.Errorf("expected exactly 1 duplicate result, got %d", duplicateCount)
	}

	var eventCount, outboxCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM events WHERE idempotency_key = $1`, idempotencyKey).Scan(&eventCount); err != nil {
		t.Fatalf("failed to count events rows: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM outbox WHERE event_id IN (SELECT event_id FROM events WHERE idempotency_key = $1)`, idempotencyKey).Scan(&outboxCount); err != nil {
		t.Fatalf("failed to count outbox rows: %v", err)
	}

	if eventCount != 1 {
		t.Errorf("expected exactly 1 events row, got %d", eventCount)
	}
	if outboxCount != 1 {
		t.Errorf("expected exactly 1 outbox row, got %d", outboxCount)
	}
}
