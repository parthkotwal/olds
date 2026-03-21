package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/olds/backend/internal/migrations"
)

// runMigrations applies any pending SQL migrations from the embedded filesystem.
//
// It is idempotent: running it multiple times is safe. golang-migrate tracks
// which migrations have been applied in a schema_migrations table it creates
// automatically. If no new migrations exist, migrate.ErrNoChange is returned
// and silently swallowed — the schema is already up to date.
//
// The function uses database/sql (not pgxpool) because golang-migrate's driver
// is built on top of the standard library interface. The pgx stdlib shim
// (jackc/pgx/v5/stdlib) registers pgx as a database/sql driver named "pgx",
// so we can open a sql.DB with sql.Open("pgx", url). This sql.DB is only used
// for migrations and closed immediately after — the long-lived pgxpool.Pool
// created in main() handles all normal queries.
func runMigrations(databaseURL string) error {
	// Open a database/sql connection for the migration runner.
	// "pgx" is the driver name registered by jackc/pgx/v5/stdlib's init().
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open sql.DB for migrations: %w", err)
	}
	defer db.Close()

	// Create the golang-migrate database driver for pgx v5.
	// WithInstance wraps our sql.DB so migrate knows how to apply SQL
	// and track migration state in the schema_migrations table.
	dbDriver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
	if err != nil {
		return fmt.Errorf("create migration db driver: %w", err)
	}

	// Create the golang-migrate source driver from the embedded filesystem.
	// migrations.FS is the embed.FS defined in internal/migrations/migrations.go.
	// The second argument is the directory within the FS — since *.sql files
	// sit at the root of the embedded FS, "." is the correct path.
	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("create migration source driver: %w", err)
	}

	// Build the migrator. "iofs" and "pgx5" are the registered driver names
	// (pgx5 is registered by golang-migrate/database/pgx/v5's init function).
	m, err := migrate.NewWithInstance("iofs", sourceDriver, "pgx5", dbDriver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	// Apply all pending migrations. ErrNoChange means the schema is already
	// up to date — not an error from our perspective.
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	if dirty {
		log.Printf("WARNING: migration version %d is dirty — a previous migration may have failed mid-run", version)
	} else {
		log.Printf("migrations applied, current schema version: %d", version)
	}

	return nil
}
