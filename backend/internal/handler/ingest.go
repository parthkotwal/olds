package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/olds/backend/internal/article"
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

// RunStartupIngest performs the same fetch → enrich → store pipeline as the
// HTTP Ingest handler, but without a Gin context. Called from main.go's
// startup goroutine so the background ingestion also benefits from ML enrichment.
//
// Separating this from Ingest avoids duplicating logic: both paths call the
// same enrich() helper. This mirrors the separation of concerns you'd use in
// Python: a service function (RunStartupIngest) vs a view function (Ingest)
// that both delegate to the same business logic.
func (h *ArticleHandler) RunStartupIngest() error {
	articles, err := h.client.FetchAll()
	if err != nil {
		return fmt.Errorf("startup ingest: fetch failed: %w", err)
	}

	if h.guardianClient != nil {
		guardianArticles, err := h.guardianClient.FetchAll()
		if err != nil {
			// Non-fatal: log and continue with NewsAPI articles only.
			log.Printf("startup ingest: guardian fetch failed: %v", err)
		} else {
			articles = append(articles, guardianArticles...)
			log.Printf("startup ingest: fetched %d guardian articles", len(guardianArticles))
		}
	}

	enriched := enrich(articles, h)
	h.store.Add(enriched)
	h.graph.Add(enriched)

	// Persist to Postgres. context.Background() is used here because
	// RunStartupIngest has no HTTP request context — it runs in a goroutine
	// launched from main(). context.Background() is the idiomatic Go root
	// context for work not associated with a specific request.
	if err := h.articleRepo.UpsertBatch(context.Background(), enriched); err != nil {
		log.Printf("startup ingest: DB persist failed: %v", err)
	}

	log.Printf("startup ingest: %d articles stored, graph: %d nodes / %d edges",
		len(enriched), h.graph.NodeCount(), h.graph.EdgeCount())
	return nil
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

			// Skip enrichment for articles with no text — the ML service
			// would return an empty entity list and a meaningless zero vector.
			if art.RawText == "" {
				log.Printf("ingest: skipping enrichment for %q (no raw text)", art.ID)
				enriched[idx] = art
				return
			}

			entities, embedding, err := h.mlClient.Analyze(art.ID, art.RawText)
			if err != nil {
				// Log the error but store the article anyway. Partial enrichment
				// beats losing the article entirely. The graph (Phase 5) will
				// simply not create edges for unenriched articles.
				log.Printf("ingest: ML enrichment failed for %q: %v", art.ID, err)
				enriched[idx] = art
				return
			}

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
