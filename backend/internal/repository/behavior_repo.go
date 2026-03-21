package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olds/backend/internal/behavior"
)

// BehaviorRepository defines the persistence operations for behavioral events.
type BehaviorRepository interface {
	// RecordEvent persists a single behavioral event to Postgres.
	// Called asynchronously (in a goroutine) from the RecordBehavior handler
	// so that DB latency never slows down the response to the frontend.
	RecordEvent(ctx context.Context, e behavior.Event) error

	// LoadSignals returns accumulated signals per article, built by aggregating
	// the raw behavior_events table. Called once at startup.
	LoadSignals(ctx context.Context) (map[string]behavior.ArticleSignals, error)
}

// pgxBehaviorRepo is the pgx-backed implementation of BehaviorRepository.
type pgxBehaviorRepo struct {
	pool *pgxpool.Pool
}

// NewBehaviorRepository returns a BehaviorRepository backed by the given connection pool.
func NewBehaviorRepository(pool *pgxpool.Pool) BehaviorRepository {
	return &pgxBehaviorRepo{pool: pool}
}

// RecordEvent inserts a single behavior event row.
//
// Events are immutable facts — we INSERT, never UPDATE. This means the
// behavior_events table is append-only, which is the right model for an
// audit-style signal log. LoadSignals aggregates them at startup.
func (r *pgxBehaviorRepo) RecordEvent(ctx context.Context, e behavior.Event) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO behavior_events (article_id, event_type, value)
		VALUES ($1, $2, $3)
	`, e.ArticleID, e.Type, e.Value)
	if err != nil {
		return fmt.Errorf("insert behavior event for article %s: %w", e.ArticleID, err)
	}
	return nil
}

// LoadSignals aggregates the raw behavior_events table into ArticleSignals per article.
//
// This produces the same state that behavior.Store would have accumulated if it
// had been running continuously since the events were first recorded — so the
// re-loaded in-memory store behaves identically to a never-restarted one.
//
// SQL breakdown:
//   - SUM(CASE WHEN type = 'dwell' THEN value ELSE 0 END): total dwell seconds
//   - MAX(CASE WHEN type = 'scroll_depth' THEN value ELSE 0 END): high-water mark
//   - COUNT(CASE WHEN type = 'reopen' THEN 1 END): number of opens
//
// COALESCE(..., 0) ensures NULL becomes 0 for articles with no events of a given type.
func (r *pgxBehaviorRepo) LoadSignals(ctx context.Context) (map[string]behavior.ArticleSignals, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			article_id,
			COALESCE(SUM(CASE WHEN event_type = 'dwell'        THEN value ELSE 0 END), 0) AS total_dwell,
			COALESCE(MAX(CASE WHEN event_type = 'scroll_depth' THEN value ELSE 0 END), 0) AS max_scroll_depth,
			COUNT(CASE WHEN event_type = 'reopen' THEN 1 END)::int                        AS open_count
		FROM behavior_events
		GROUP BY article_id
	`)
	if err != nil {
		return nil, fmt.Errorf("query behavior signals: %w", err)
	}
	defer rows.Close()

	signals := make(map[string]behavior.ArticleSignals)
	for rows.Next() {
		var articleID string
		var sig behavior.ArticleSignals
		if err := rows.Scan(&articleID, &sig.TotalDwell, &sig.MaxScrollDepth, &sig.OpenCount); err != nil {
			return nil, fmt.Errorf("scan behavior signal row: %w", err)
		}
		signals[articleID] = sig
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate behavior signal rows: %w", err)
	}

	return signals, nil
}
