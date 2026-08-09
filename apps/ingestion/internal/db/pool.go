// Package db provides shared database connection setup used by every
// Relay service, so pool limits are configured identically everywhere
// rather than duplicated per service.
package db

import (
	"database/sql"
	"time"
)

const (
	maxOpenConns    = 10
	maxIdleConns    = 5
	connMaxLifetime = 30 * time.Minute
)

// Open opens a Postgres connection pool with Relay's standard pool limits
// applied.
func Open(driverName, dataSourceName string) (*sql.DB, error) {
	conn, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}

	conn.SetMaxOpenConns(maxOpenConns)
	conn.SetMaxIdleConns(maxIdleConns)
	conn.SetConnMaxLifetime(connMaxLifetime)

	return conn, nil
}
