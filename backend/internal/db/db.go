// Package db provides the Postgres connection pool for the Olds backend.
//
// In Go, a "package" is a directory — every .go file in this directory shares
// the package name "db" and can access each other's unexported identifiers.
// This package has one job: create a validated pgx connection pool. All
// application-level database logic (queries, transactions) lives in the
// repository package instead.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	pgxvector "github.com/pgvector/pgvector-go/pgx"
)

// Open creates a pgx connection pool from the given Postgres connection string,
// pings the database to verify connectivity, and registers the pgvector type
// so that vector(384) columns can be scanned into pgvector.Vector values.
//
// Callers should defer pool.Close() when the application exits:
//
//	pool, err := db.Open(ctx, databaseURL)
//	if err != nil { log.Fatal(err) }
//	defer pool.Close()
//
// Go's pgxpool.Pool is a connection pool: one pool per process, shared across
// all goroutines. Each goroutine that needs a connection calls Acquire() (or
// implicitly acquires one through Query/Exec), and the pool manages checkout
// and return automatically. This is different from Python where you often open
// one connection per request — in Go the pool is the idiomatic pattern.
func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	// pgxpool.New parses the connection string, creates the pool configuration,
	// and establishes an initial set of connections. The pool is ready to use
	// immediately — you do not call Connect() separately.
	//
	// The connection string format: postgres://user:password@host:port/dbname?options
	// pgx also accepts the libpq keyword=value format, but URL form is simpler.
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	// Ping verifies we can actually reach the database. Fail fast here rather
	// than discovering the database is unreachable on the first query.
	//
	// fmt.Errorf("...: %w", err) is Go's idiom for wrapping errors. The %w
	// verb (unlike %v) preserves the original error for inspection with
	// errors.Is() / errors.As(). This is like Python's "raise X from Y".
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Register the pgvector type with pgx's type system.
	//
	// pgvector stores vectors as a custom Postgres type. By default pgx doesn't
	// know how to scan it into a Go value. RegisterTypes tells pgx to decode
	// vector columns into pgvector.Vector (which wraps []float32). Without this
	// call, scanning a vector(384) column would panic with "unknown OID".
	//
	// We acquire a single connection just for registration. After conn.Release()
	// it returns to the pool. All connections in the pool share the same type
	// map, so this one-time registration covers all future queries.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("acquire connection for type registration: %w", err)
	}
	defer conn.Release()

	if err := pgxvector.RegisterTypes(ctx, conn.Conn()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("register pgvector types: %w", err)
	}

	return pool, nil
}
