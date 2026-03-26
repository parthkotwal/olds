package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/olds/backend/internal/article"
	"github.com/olds/backend/internal/repository"
)

// Ingest handles POST /ingest.
// It triggers a fresh fetch from NewsAPI, enriches each article via the ML
// service (if available), and loads the results into the store.
//
// This endpoint exists for development convenience — you can re-populate the
// store without restarting the server.
//
// This is a method on ArticleHandler (defined in articles.go), so it has
// access to h.store, h.client, and h.mlClient. Multiple files can define
// methods on the same type as long as they share a package.
func (h *ArticleHandler) Ingest(c *gin.Context) {
	articles, err := h.client.FetchAll()
	if err != nil {
		// 502 Bad Gateway: our server is up, but upstream (NewsAPI) failed.
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	// Also fetch from The Guardian if the client is configured.
	// Guardian articles are appended to the same slice — the enrichment
	// pipeline and graph treat them identically to NewsAPI articles.
	if h.guardianClient != nil {
		guardianArticles, err := h.guardianClient.FetchAll()
		if err != nil {
			// Log but don't abort — partial results from NewsAPI are still useful.
			log.Printf("ingest: guardian fetch failed: %v", err)
		} else {
			articles = append(articles, guardianArticles...)
		}
	}

	// Enrich articles with ML data (entities + embedding) if the ML client
	// is configured. Enrichment is best-effort — failure logs a warning but
	// does not prevent the article from being stored.
	enriched := enrich(articles, h)

	h.store.Add(enriched)
	h.graph.Add(enriched)

	// Persist to Postgres. Non-fatal: in-memory stores are already updated,
	// so the feed is live even if the DB write fails. Log and continue.
	if err := h.articleRepo.UpsertBatch(c.Request.Context(), enriched); err != nil {
		log.Printf("ingest: DB persist failed: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"ingested":    len(enriched),
		"total":       h.store.Count(),
		"graph_nodes": h.graph.NodeCount(),
		"graph_edges": h.graph.EdgeCount(),
	})
}

// RunScheduledIngest performs the full fetch → enrich → store pipeline without
// a Gin context. Used for both the initial startup run and subsequent ticker
// runs (every 30 minutes by default). Separating this from the HTTP Ingest
// handler keeps the business logic reusable without a request context.
//
// After each run it updates the ArticleHandler's ingestion telemetry fields
// so the /stats endpoint can report on ingestion health.
func (h *ArticleHandler) RunScheduledIngest() error {
	// Time the full ingest cycle (fetch → enrich → graph.Add) for /stats.
	ingestStart := time.Now()

	articles, err := h.client.FetchAll()
	if err != nil {
		return fmt.Errorf("scheduled ingest: NewsAPI fetch failed: %w", err)
	}

	if h.guardianClient != nil {
		guardianArticles, err := h.guardianClient.FetchAll()
		if err != nil {
			log.Printf("scheduled ingest: guardian fetch failed (continuing): %v", err)
		} else {
			articles = append(articles, guardianArticles...)
			log.Printf("scheduled ingest: fetched %d guardian articles", len(guardianArticles))
		}
	}

	enriched := enrich(articles, h)
	h.store.Add(enriched)
	h.graph.Add(enriched)

	// Record full-cycle latency now that the graph is updated.
	h.ingestTotalTimings.Add(time.Since(ingestStart))

	// context.Background() is the idiomatic root context for work not tied
	// to an HTTP request — this goroutine has no request lifecycle to cancel it.
	if err := h.articleRepo.UpsertBatch(context.Background(), enriched); err != nil {
		log.Printf("scheduled ingest: DB persist failed: %v", err)
	}

	// Log entity quality metrics — the primary stress-test signal for whether
	// the ML pipeline is producing useful signal. Key things to watch:
	//   - noEntityCount high → ML service struggling, graph edges will be
	//     purely cosine-based (lower precision for cross-topic discovery)
	//   - GPE/LOC count high → location entities may create noisy connections
	//     between unrelated stories set in the same country (Phase 14 finding)
	logEntityQuality(enriched)

	graphStats := h.graph.Stats()
	log.Printf("scheduled ingest complete: %d articles stored | graph: %d nodes / %d unique edges (%.1f%% density)",
		len(enriched), graphStats.NodeCount, graphStats.UniqueEdges, graphStats.DensityPct)

	// Update telemetry under lock — the /stats handler reads these from the
	// main goroutine concurrently with ingestion goroutine writes.
	now := time.Now()
	h.ingestMu.Lock()
	h.ingestRunCount++
	h.lastIngestAt = now
	h.lastIngestCount = len(enriched)
	runCount := h.ingestRunCount
	// Append per-run entry for 7-day rolling average, bounded to last 500 runs.
	h.ingestHistory = append(h.ingestHistory, ingestRun{at: now, count: len(enriched)})
	if len(h.ingestHistory) > 500 {
		h.ingestHistory = h.ingestHistory[len(h.ingestHistory)-500:]
	}
	h.ingestMu.Unlock()

	// Persist a snapshot row so the stress-test history is queryable later.
	// Non-fatal: if the DB write fails, the in-memory state is still correct.
	decay := h.store.DecaySnapshot()
	snap := repository.Snapshot{
		NodeCount:          graphStats.NodeCount,
		UniqueEdges:        graphStats.UniqueEdges,
		DensityPct:         graphStats.DensityPct,
		AvgEdgesPerNode:    graphStats.AvgEdgesPerNode,
		IsolatedNodes:      graphStats.IsolatedNodes,
		MaxEdgesPerNode:    graphStats.MaxEdgesPerNode,
		CrossTopicRatioPct: graphStats.CrossTopicRatioPct,
		ArticlesFresh:      decay.Fresh,
		ArticlesRecent:     decay.Recent,
		ArticlesAging:      decay.Aging,
		ArticlesStale:      decay.Stale,
		IngestRunCount:     runCount,
		LastIngestArticles: len(enriched),
	}
	if err := h.snapshotRepo.Save(context.Background(), snap); err != nil {
		log.Printf("scheduled ingest: snapshot save failed: %v", err)
	}

	return nil
}

// logEntityQuality prints a breakdown of entity types and coverage after each
// ingestion run. Helps identify whether GPE/LOC entities are dominating and
// creating low-quality geographic connections (a known stress-test risk).
func logEntityQuality(articles []article.Article) {
	var totalEntities, noEntityCount int
	typeCounts := make(map[string]int)

	for _, a := range articles {
		if len(a.Entities) == 0 {
			noEntityCount++
			continue
		}
		for _, e := range a.Entities {
			typeCounts[e.Label]++
			totalEntities++
		}
	}

	log.Printf("ingest entity quality: %d entities across %d articles (%d with none)",
		totalEntities, len(articles), noEntityCount)
	// Log each entity type count so we can spot GPE/LOC dominance.
	for label, count := range typeCounts {
		pct := float64(count) / float64(totalEntities) * 100
		log.Printf("  %s: %d (%.0f%%)", label, count, pct)
	}
}

// enrich calls the ML service for each article concurrently and returns a
// new slice with Entities and Embedding populated where the call succeeded.
//
// ── Why sync.WaitGroup? ──────────────────────────────────────────────────────
// Each ML call is an independent HTTP round-trip (~100–300ms). Doing them
// sequentially would make ingesting 80 articles take ~20 seconds. With
// goroutines + WaitGroup, all calls happen in parallel and the total wait
// time equals the slowest single call.
//
// The WaitGroup pattern:
//   1. wg.Add(n)  — declare how many goroutines you are about to launch
//   2. go func()  — launch the goroutine
//   3. defer wg.Done() inside each goroutine — decrements the counter on exit
//   4. wg.Wait()  — block until all goroutines have called Done()
//
// This is Go's version of Promise.all() in JavaScript or asyncio.gather() in Python.
//
// ── Why write to slice indices instead of using a mutex + append? ─────────────
// We pre-allocate enriched as a slice of length len(articles), then each
// goroutine writes to its own index (enriched[i] = ...). Writing to distinct
// slice indices from separate goroutines is safe without a mutex — there is no
// overlap. Using append to a shared slice WOULD require a mutex because append
// may reallocate the backing array.
func enrich(articles []article.Article, h *ArticleHandler) []article.Article {
	// If no ML client is configured, return articles as-is.
	if h.mlClient == nil {
		log.Println("ingest: ML_SERVICE_URL not set — skipping enrichment")
		return articles
	}

	// Pre-allocate result slice with the same length as input.
	// Each goroutine writes to a unique index, so no mutex is needed.
	enriched := make([]article.Article, len(articles))

	// sem is a semaphore implemented as a buffered channel. Sending a token
	// acquires a slot; receiving one releases it. This caps concurrent ML
	// requests at 5, preventing the sentence-transformer from receiving a
	// flood of parallel inference calls that would spike its memory usage
	// and trigger an OOM kill on Railway.
	//
	// In Go, a buffered channel of capacity N is the idiomatic semaphore —
	// no external library needed.
	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup

	for i, a := range articles {
		wg.Add(1)

		// IMPORTANT: capture i and a as local variables for the goroutine.
		// In Go (before 1.22), range loop variables are reused each iteration.
		// If we captured them by reference, every goroutine would see the LAST
		// iteration's values by the time they run — a classic Go footgun.
		// Passing them as function arguments creates a fresh copy per goroutine.
		// (Go 1.22+ fixes this by giving each iteration its own variable, but
		// passing arguments explicitly is clearer and works on all versions.)
		go func(idx int, art article.Article) {
			defer wg.Done()

			// Acquire semaphore slot before calling the ML service.
			// This blocks until fewer than maxConcurrent calls are in flight.
			sem <- struct{}{}
			defer func() { <-sem }()

			// Skip enrichment for articles with no text — the ML service
			// would return an empty entity list and a meaningless zero vector.
			if art.RawText == "" {
				log.Printf("ingest: skipping enrichment for %q (no raw text)", art.ID)
				enriched[idx] = art
				return
			}

			// Time the ML service call and track success/failure for /stats.
			mlStart := time.Now()
			entities, embedding, err := h.mlClient.Analyze(art.ID, art.RawText)
			h.mlInferTimings.Add(time.Since(mlStart))
			atomic.AddInt64(&h.mlAttempts, 1)

			if err != nil {
				// Log the error but store the article anyway. Partial enrichment
				// beats losing the article entirely. The graph (Phase 5) will
				// simply not create edges for unenriched articles.
				log.Printf("ingest: ML enrichment failed for %q: %v", art.ID, err)
				enriched[idx] = art
				return
			}
			atomic.AddInt64(&h.mlSuccesses, 1)

			// In Go, structs are value types — assigning art makes a copy.
			// We modify the copy and store it at enriched[idx]. The original
			// `art` parameter is unchanged. This is different from Python,
			// where assigning a dict gives you a reference, not a copy.
			art.Entities = entities
			art.Embedding = embedding
			enriched[idx] = art

			log.Printf("ingest: enriched %q — %d entities, %d-dim embedding",
				art.ID, len(entities), len(embedding))
		}(i, a)
	}

	// Block until all goroutines have finished (all wg.Done() calls received).
	wg.Wait()

	return enriched
}
