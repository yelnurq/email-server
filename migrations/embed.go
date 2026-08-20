// Package migrations embeds SQL migration files so cmd/migrate ships as a
// single cross-platform binary with no external file dependencies.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
