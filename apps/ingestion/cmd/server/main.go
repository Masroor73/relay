// Command server starts the Relay ingestion HTTP service.
package main

import (
	"database/sql"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Masroor73/relay/apps/ingestion/internal/config"
	"github.com/Masroor73/relay/apps/ingestion/internal/logging"
	"github.com/Masroor73/relay/apps/ingestion/internal/server"
)

const (
	maxOpenConns    = 10
	maxIdleConns    = 5
	connMaxLifetime = 30 * time.Minute
)

func main() {
	logger := logging.New()
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to open database connection", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	srv := server.New(cfg, db)

	slog.Info("ingestion service starting", "port", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
