package server

import (
	"database/sql"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Masroor73/relay/apps/ingestion/internal/stripewebhook"
)

const pgUniqueViolation = "23505"

func stripeWebhookHandler(db *sql.DB, webhookSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}

		signatureHeader := r.Header.Get("Stripe-Signature")
		event, err := stripewebhook.VerifySignature(body, signatureHeader, webhookSecret)
		if err != nil {
			log.Printf("event_id=unknown signature verification failed: %v", err)
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		_, err = db.Exec(
			`INSERT INTO events (idempotency_key, source, payload) VALUES ($1, $2, $3)`,
			event.ID, "stripe", body,
		)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
				log.Printf("event_id=%s duplicate delivery, already recorded", event.ID)
				w.WriteHeader(http.StatusOK)
				return
			}
			log.Printf("event_id=%s failed to persist event: %v", event.ID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		log.Printf("event_id=%s accepted", event.ID)
		w.WriteHeader(http.StatusOK)
	}
}
