package repository

// snapshot_repo.go persists and loads graph_snapshots rows.
//
// A snapshot is saved after every scheduled ingestion run, capturing the
// full system state at that moment. This creates a time series that lets us
// observe how the graph evolves over the Phase 14 stress-test period —
// things like density growth, decay distribution, and cross-topic ratio
// that are invisible from a single point-in-time /stats call.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Snapshot holds a full system metrics snapshot at a point in time.
// Its fields mirror the graph_snapshots table columns exactly.
type Snapshot struct {
	ID                 int64     `json:"id"`
	CapturedAt         time.Time `json:"captured_at"`
	NodeCount          int       `json:"node_count"`
	UniqueEdges        int       `json:"unique_edges"`
	DensityPct         float64   `json:"density_pct"`
	AvgEdgesPerNode    float64   `json:"avg_edges_per_node"`
	IsolatedNodes      int       `json:"isolated_nodes"`
	MaxEdgesPerNode    int       `json:"max_edges_per_node"`
	CrossTopicRatioPct float64   `json:"cross_topic_ratio_pct"`
	ArticlesFresh      int       `json:"articles_fresh"`
	ArticlesRecent     int       `json:"articles_recent"`
	ArticlesAging      int       `json:"articles_aging"`
	ArticlesStale      int       `json:"articles_stale"`
	IngestRunCount     int       `json:"ingest_run_count"`
	LastIngestArticles int       `json:"last_ingest_articles"`
}

// SnapshotRepository defines the persistence operations for graph snapshots.
type SnapshotRepository interface {
	// Save inserts one snapshot row. Called after every ingestion run.
	Save(ctx context.Context, s Snapshot) error

	// LoadRecent returns the most recent `limit` snapshots, newest first.
	// Used by GET /stats/history to return the time series.
	LoadRecent(ctx context.Context, limit int) ([]Snapshot, error)
}

// pgxSnapshotRepo is the pgx-backed implementation.
type pgxSnapshotRepo struct {
	pool *pgxpool.Pool
}

// NewSnapshotRepository returns a SnapshotRepository backed by the given pool.
func NewSnapshotRepository(pool *pgxpool.Pool) SnapshotRepository {
	return &pgxSnapshotRepo{pool: pool}
}

func (r *pgxSnapshotRepo) Save(ctx context.Context, s Snapshot) error {
	const q = `
		INSERT INTO graph_snapshots (
			node_count, unique_edges, density_pct, avg_edges_per_node,
			isolated_nodes, max_edges_per_node, cross_topic_ratio_pct,
			articles_fresh, articles_recent, articles_aging, articles_stale,
			ingest_run_count, last_ingest_articles
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10, $11,
			$12, $13
		)`

	_, err := r.pool.Exec(ctx, q,
		s.NodeCount, s.UniqueEdges, s.DensityPct, s.AvgEdgesPerNode,
		s.IsolatedNodes, s.MaxEdgesPerNode, s.CrossTopicRatioPct,
		s.ArticlesFresh, s.ArticlesRecent, s.ArticlesAging, s.ArticlesStale,
		s.IngestRunCount, s.LastIngestArticles,
	)
	if err != nil {
		return fmt.Errorf("snapshot save: %w", err)
	}
	return nil
}

func (r *pgxSnapshotRepo) LoadRecent(ctx context.Context, limit int) ([]Snapshot, error) {
	const q = `
		SELECT
			id, captured_at,
			node_count, unique_edges, density_pct, avg_edges_per_node,
			isolated_nodes, max_edges_per_node, cross_topic_ratio_pct,
			articles_fresh, articles_recent, articles_aging, articles_stale,
			ingest_run_count, last_ingest_articles
		FROM graph_snapshots
		ORDER BY captured_at DESC
		LIMIT $1`

	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("snapshot load: %w", err)
	}
	defer rows.Close()

	var snapshots []Snapshot
	for rows.Next() {
		var s Snapshot
		if err := rows.Scan(
			&s.ID, &s.CapturedAt,
			&s.NodeCount, &s.UniqueEdges, &s.DensityPct, &s.AvgEdgesPerNode,
			&s.IsolatedNodes, &s.MaxEdgesPerNode, &s.CrossTopicRatioPct,
			&s.ArticlesFresh, &s.ArticlesRecent, &s.ArticlesAging, &s.ArticlesStale,
			&s.IngestRunCount, &s.LastIngestArticles,
		); err != nil {
			return nil, fmt.Errorf("snapshot scan: %w", err)
		}
		snapshots = append(snapshots, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("snapshot rows: %w", err)
	}

	// Return empty slice rather than nil so JSON encodes as [] not null.
	if snapshots == nil {
		snapshots = []Snapshot{}
	}
	return snapshots, nil
}
