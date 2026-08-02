// Command migrate applies pending database migrations. It reads
// DATABASE_URL and works unchanged against local Docker Postgres or Neon.
package main

import (
	"database/sql"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Masroor73/relay/apps/ingestion/internal/migrations"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	direction := "up"
	if len(os.Args) > 1 {
		direction = os.Args[1]
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("failed to open database connection: %v", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatalf("failed to create migration driver: %v", err)
	}

	source, err := iofs.New(migrations.FS, "files")
	if err != nil {
		log.Fatalf("failed to load migration source: %v", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		log.Fatalf("failed to initialize migrator: %v", err)
	}

	switch direction {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	default:
		log.Fatalf("unknown direction %q: expected \"up\" or \"down\"", direction)
	}

	if err != nil && err != migrate.ErrNoChange {
		log.Fatalf("migration failed: %v", err)
	}

	log.Println("migrations applied successfully")
}
