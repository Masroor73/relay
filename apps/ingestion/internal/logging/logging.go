// Package logging provides the shared structured logger used across all
// Relay services. Every log line is JSON, and any line touching a specific
// event must include an event_id field — this is the one required
// convention that carries through every remaining milestone.
package logging

import (
	"log/slog"
	"os"
)

// New returns a JSON-structured logger writing to stdout.
func New() *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, nil)
	return slog.New(handler)
}
