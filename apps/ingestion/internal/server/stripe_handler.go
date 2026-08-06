package server

import (
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/Masroor73/relay/apps/ingestion/internal/eventstore"
	"github.com/Masroor73/relay/apps/ingestion/internal/stripewebhook"
)

const maxWebhookBodyBytes = 1 << 20 // 1MB — comfortably covers real Stripe payloads while blocking abuse

func stripeWebhookHandler(db *sql.DB, webhookSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			slog.Warn("request body error", "event_id", "unknown", "error", err)
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		signatureHeader := r.Header.Get("Stripe-Signature")
		event, err := stripewebhook.VerifySignature(body, signatureHeader, webhookSecret)
		if err != nil {
			slog.Warn("signature verification failed", "event_id", "unknown", "error", err)
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		err = eventstore.Insert(r.Context(), db, event.ID, "stripe", body)
		if err != nil {
			if errors.Is(err, eventstore.ErrDuplicateEvent) {
				slog.Info("duplicate delivery, already recorded", "event_id", event.ID)
				w.WriteHeader(http.StatusOK)
				return
			}
			slog.Error("failed to persist event", "event_id", event.ID, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		slog.Info("event accepted", "event_id", event.ID)
		w.WriteHeader(http.StatusOK)
	}
}
