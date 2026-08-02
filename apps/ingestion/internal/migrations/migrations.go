// Package migrations embeds the SQL migration files directly into the
// compiled binary, so the migration runner has no external file
// dependency at deploy time.
package migrations

import "embed"

//go:embed files/*.sql
var FS embed.FS
