// Package server builds and configures the ingestion service's HTTP server.
package server

import (
	"net/http"

	"github.com/Masroor73/relay/apps/ingestion/internal/config"
)

// New constructs an HTTP server configured with the ingestion service's routes.
func New(cfg *config.Config) *http.Server {
	mux := http.NewServeMux()

	return &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}
}
