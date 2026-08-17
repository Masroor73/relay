package server

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// dashboardListLimit caps how many rows a single list request returns.
// No pagination yet — acceptable at portfolio scale; revisit if the
// dashboard ever needs to page through more than this in one view.
const dashboardListLimit = 100

type eventResponse struct {
	EventID        string    `json:"event_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Source         string    `json:"source"`
	Status         string    `json:"status"`
	ReceivedAt     time.Time `json:"received_at"`
}

type orderResponse struct {
	OrderID       string    `json:"order_id"`
	StripeEventID string    `json:"stripe_event_id"`
	AmountCents   int64     `json:"amount_cents"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type dlqResponse struct {
	DLQID      string    `json:"dlq_id"`
	EventID    string    `json:"event_id"`
	FinalError string    `json:"final_error"`
	MovedAt    time.Time `json:"moved_at"`
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

func listEventsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(),
			`SELECT event_id, idempotency_key, source, status, received_at
			 FROM events ORDER BY received_at DESC LIMIT $1`, dashboardListLimit)
		if err != nil {
			slog.Error("failed to query events", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		results := []eventResponse{}
		for rows.Next() {
			var e eventResponse
			if err := rows.Scan(&e.EventID, &e.IdempotencyKey, &e.Source, &e.Status, &e.ReceivedAt); err != nil {
				slog.Error("failed to scan event row", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			results = append(results, e)
		}
		if err := rows.Err(); err != nil {
			slog.Error("error iterating event rows", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, results)
	}
}

func listOrdersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(),
			`SELECT order_id, stripe_event_id, amount_cents, status, created_at
			 FROM orders ORDER BY created_at DESC LIMIT $1`, dashboardListLimit)
		if err != nil {
			slog.Error("failed to query orders", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		results := []orderResponse{}
		for rows.Next() {
			var o orderResponse
			if err := rows.Scan(&o.OrderID, &o.StripeEventID, &o.AmountCents, &o.Status, &o.CreatedAt); err != nil {
				slog.Error("failed to scan order row", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			results = append(results, o)
		}
		if err := rows.Err(); err != nil {
			slog.Error("error iterating order rows", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, results)
	}
}

func listDLQHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(),
			`SELECT dlq_id, event_id, final_error, moved_at
			 FROM dead_letter_events ORDER BY moved_at DESC LIMIT $1`, dashboardListLimit)
		if err != nil {
			slog.Error("failed to query dead_letter_events", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		results := []dlqResponse{}
		for rows.Next() {
			var d dlqResponse
			if err := rows.Scan(&d.DLQID, &d.EventID, &d.FinalError, &d.MovedAt); err != nil {
				slog.Error("failed to scan dead-letter row", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			results = append(results, d)
		}
		if err := rows.Err(); err != nil {
			slog.Error("error iterating dead-letter rows", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, results)
	}
}
