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
	limiter := newIPRateLimiter()

	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /webhooks/stripe", rateLimitMiddleware(limiter, stripeWebhookHandler(db, cfg.StripeWebhookSecret)))

	return &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}
}
