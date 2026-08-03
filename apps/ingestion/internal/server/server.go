// Package server builds and configures the ingestion service's HTTP server.
package server

import (
	"database/sql"
	"net/http"

	"github.com/Masroor73/relay/apps/ingestion/internal/config"
)

// New constructs an HTTP server configured with the ingestion service's routes.
func New(cfg *config.Config, db *sql.DB) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /webhooks/stripe", stripeWebhookHandler(db, cfg.StripeWebhookSecret))

	return &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}
}
