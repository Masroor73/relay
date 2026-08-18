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

	// Dashboard read endpoints - unauthenticated for now. ENG-35 adds the
	// shared-password auth middleware these will be wrapped in; shipping
	// them separately mirrors how /webhooks/stripe itself landed (ENG-9)
	// before its own protections (ENG-11/ENG-12) were added.
	mux.HandleFunc("GET /api/events", listEventsHandler(db))
	mux.HandleFunc("GET /api/orders", listOrdersHandler(db))
	mux.HandleFunc("GET /api/dlq", listDLQHandler(db))
	mux.HandleFunc("POST /api/dlq/{id}/replay", replayDLQHandler(db))

	return &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}
}
