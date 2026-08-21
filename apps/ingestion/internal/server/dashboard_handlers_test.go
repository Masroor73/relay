package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func testDB(t *testing.T) *sql.DB {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test that requires a real Postgres connection")
	}
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("failed to open database connection: %v", err)
	}
	return db
}

func TestListEventsHandler_ReturnsSeededEvent(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	const idempotencyKey = "evt_list_test"
	cleanup := func() {
		if _, err := db.Exec(`DELETE FROM events WHERE idempotency_key = $1`, idempotencyKey); err != nil {
			t.Logf("cleanup: failed to delete events row: %v", err)
		}
	}
	cleanup()
	defer cleanup()

	if _, err := db.Exec(
		`INSERT INTO events (idempotency_key, source, payload) VALUES ($1, $2, $3)`,
		idempotencyKey, "stripe", []byte(`{"id":"evt_list_test"}`),
	); err != nil {
		t.Fatalf("failed to insert test event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	rec := httptest.NewRecorder()
	listEventsHandler(db)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), idempotencyKey) {
		t.Errorf("expected response to contain seeded event %q, got: %s", idempotencyKey, rec.Body.String())
	}
}

func TestReplayDLQHandler_MovesEventBackToOutbox(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	const idempotencyKey = "evt_replay_handler_test"

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

	var eventID string
	err := db.QueryRow(
		`INSERT INTO events (idempotency_key, source, payload) VALUES ($1, $2, $3) RETURNING event_id`,
		idempotencyKey, "stripe", []byte(`{"id":"evt_replay_handler_test"}`),
	).Scan(&eventID)
	if err != nil {
		t.Fatalf("failed to insert test event: %v", err)
	}

	var dlqID string
	err = db.QueryRow(
		`INSERT INTO dead_letter_events (event_id, final_error) VALUES ($1, $2) RETURNING dlq_id`,
		eventID, "test error for replay handler test",
	).Scan(&dlqID)
	if err != nil {
		t.Fatalf("failed to insert test dead-letter row: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/dlq/"+dlqID+"/replay", nil)
	req.SetPathValue("id", dlqID)
	rec := httptest.NewRecorder()
	replayDLQHandler(db)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var dlqCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM dead_letter_events WHERE dlq_id = $1`, dlqID).Scan(&dlqCount); err != nil {
		t.Fatalf("failed to count dead_letter_events rows: %v", err)
	}
	if dlqCount != 0 {
		t.Errorf("expected dead_letter_events row to be removed, got %d remaining", dlqCount)
	}

	var attemptCount int
	err = db.QueryRow(`SELECT attempt_count FROM outbox WHERE event_id = $1`, eventID).Scan(&attemptCount)
	if err != nil {
		t.Fatalf("expected event to exist in outbox after replay, query failed: %v", err)
	}
	if attemptCount != 0 {
		t.Errorf("expected attempt_count to be reset to 0, got %d", attemptCount)
	}
}

func TestReplayDLQHandler_NotFound_Returns404(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/dlq/00000000-0000-0000-0000-000000000000/replay", nil)
	req.SetPathValue("id", "00000000-0000-0000-0000-000000000000")
	rec := httptest.NewRecorder()
	replayDLQHandler(db)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for nonexistent dlq_id, got %d", rec.Code)
	}
}
