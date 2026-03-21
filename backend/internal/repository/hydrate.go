package repository

import (
	"context"
	"fmt"
	"log"

	"github.com/olds/backend/internal/article"
	"github.com/olds/backend/internal/behavior"
	"github.com/olds/backend/internal/graph"
)

// HydrateFromDB loads all persisted data from Postgres into the in-memory
// stores and graph. It must be called synchronously at startup, before the
// HTTP server begins serving requests — this ensures the very first request
// sees the full persisted state rather than an empty feed.
//
// Hydration sequence:
//  1. Load all articles (with entities + embeddings) from Postgres
//  2. Populate article.Store — the source of truth for article data
//  3. Populate graph.Graph — recomputes all edge weights from stored vectors
//  4. Load aggregated behavior signals from Postgres
//  5. Populate behavior.Store — restores feed ranking state
//
// Why are graph edges not persisted? Edges are derived data — they are fully
// determined by the article embeddings and entity lists. Recomputing them from
// the raw data on startup means the graph always reflects the current edge
// weighting formula (which may change between versions). Storing derived data
// creates two sources of truth; recomputing is safer and fast enough at the
// current article count.
//
// Non-fatal: if hydration fails, the function logs and returns an error, but
// the server can still start with empty in-memory stores. The startup ingestion
// goroutine will repopulate the stores from the news APIs.
func HydrateFromDB(
	ctx context.Context,
	articleRepo ArticleRepository,
	behaviorRepo BehaviorRepository,
	store *article.Store,
	g *graph.Graph,
	bs *behavior.Store,
) error {
	// ── Step 1 + 2 + 3: articles → store → graph ─────────────────────────────
	articles, err := articleRepo.LoadAll(ctx)
	if err != nil {
		return fmt.Errorf("load articles from DB: %w", err)
	}

	if len(articles) > 0 {
		store.Add(articles)
		g.Add(articles)
		log.Printf("hydrate: loaded %d articles, graph: %d nodes / %d edges",
			len(articles), g.NodeCount(), g.EdgeCount())
	} else {
		log.Println("hydrate: no articles in database — starting fresh")
	}

	// ── Step 4 + 5: behavior signals → behavior store ────────────────────────
	signals, err := behaviorRepo.LoadSignals(ctx)
	if err != nil {
		// Non-fatal: feed still works without behavioral signals (falls back to
		// pure time-decay ordering). Log and continue rather than aborting startup.
		log.Printf("hydrate: failed to load behavior signals (feed will use decay-only ranking): %v", err)
		return nil
	}

	if len(signals) > 0 {
		bs.BulkLoad(signals)
		log.Printf("hydrate: loaded behavior signals for %d articles", len(signals))
	}

	return nil
}
