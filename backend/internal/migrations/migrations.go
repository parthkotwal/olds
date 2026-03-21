// Package migrations embeds the SQL migration files into the compiled binary.
//
// Keeping this in its own package lets cmd/server/migrate.go import migrations.FS
// without needing a relative path that crosses package boundaries — //go:embed
// requires the target files to be within the same directory subtree as the
// source file containing the directive.
//
// The SQL files live alongside this .go file in internal/migrations/.
// New migrations are simply added as new numbered .sql files here.
package migrations

import "embed"

// FS is the embedded filesystem containing all migration files.
// It is exported so cmd/server/migrate.go can pass it to iofs.New.
//
//go:embed *.sql
var FS embed.FS
