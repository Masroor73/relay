// Command server starts the Relay ingestion HTTP service.
package main

import (
	"log"

	"github.com/Masroor73/relay/apps/ingestion/internal/config"
	"github.com/Masroor73/relay/apps/ingestion/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	srv := server.New(cfg)

	log.Printf("ingestion service starting on port %s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
