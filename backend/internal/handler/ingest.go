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
func (h *ArticleHandler) Ingest(c *gin.Context) {
	var articles []article.Article
	var fetchErrors []string

	newsAPIArticles, err := h.client.FetchAll()
	if err != nil {
		log.Printf("ingest: NewsAPI fetch failed: %v", err)
		fetchErrors = append(fetchErrors, "newsapi: "+err.Error())
	} else {
		articles = append(articles, newsAPIArticles...)
	}

	if h.guardianClient != nil {
		guardianArticles, err := h.guardianClient.FetchAll()
		if err != nil {
			log.Printf("ingest: guardian fetch failed: %v", err)
			fetchErrors = append(fetchErrors, "guardian: "+err.Error())
		} else {
			articles = append(articles, guardianArticles...)
		}
	}

	if len(articles) == 0 {
		c.JSON(http.StatusBadGateway, gin.H{"error": "all sources failed", "details": fetchErrors})
		return
	}

	enriched := enrich(articles, h)

	h.store.Add(enriched)
	h.graph.Add(enriched)

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

// RunScheduledIngest performs the full fetch → enrich → store pipeline.
func (h *ArticleHandler) RunScheduledIngest() error {
	ingestStart := time.Now()

	var articles []article.Article

	newsAPIArticles, err := h.client.FetchAll()
	if err != nil {
		log.Printf("scheduled ingest: NewsAPI fetch failed (continuing with Guardian): %v", err)
	} else {
		articles = append(articles, newsAPIArticles...)
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

	if len(articles) == 0 {
		return fmt.Errorf("scheduled ingest: all sources failed, no articles to ingest")
	}

	enriched := enrich(articles, h)
	h.store.Add(enriched)
	h.graph.Add(enriched)

	h.ingestTotalTimings.Add(time.Since(ingestStart))

	if err := h.articleRepo.UpsertBatch(context.Background(), enriched); err != nil {
		log.Printf("scheduled ingest: DB persist failed: %v", err)
	}

	logEntityQuality(enriched)

	graphStats := h.graph.Stats()
	log.Printf("scheduled ingest complete: %d articles stored | graph: %d nodes / %d unique edges (%.1f%% density)",
		len(enriched), graphStats.NodeCount, graphStats.UniqueEdges, graphStats.DensityPct)

	now := time.Now()
	h.ingestMu.Lock()
	h.ingestRunCount++
	h.lastIngestAt = now
	h.lastIngestCount = len(enriched)
	runCount := h.ingestRunCount
	h.ingestHistory = append(h.ingestHistory, ingestRun{at: now, count: len(enriched)})
	if len(h.ingestHistory) > 500 {
		h.ingestHistory = h.ingestHistory[len(h.ingestHistory)-500:]
	}
	h.ingestMu.Unlock()

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
	for label, count := range typeCounts {
		pct := float64(count) / float64(totalEntities) * 100
		log.Printf("  %s: %d (%.0f%%)", label, count, pct)
	}
}

// enrich calls the ML service for NER and OpenAI for embeddings concurrently.
//
// For each article, two independent calls happen in parallel:
//   - ML service: entity extraction (spaCy NER)
//   - OpenAI: text-embedding-3-small (384 dimensions)
//
// Both are best-effort — an article is stored even if one or both fail.
func enrich(articles []article.Article, h *ArticleHandler) []article.Article {
	hasML := h.mlClient != nil
	hasEmbed := h.embedClient != nil

	if !hasML && !hasEmbed {
		log.Println("ingest: no ML or embedding client configured — skipping enrichment")
		return articles
	}

	enriched := make([]article.Article, len(articles))

	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup

	for i, a := range articles {
		wg.Add(1)

		go func(idx int, art article.Article) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			if art.RawText == "" {
				log.Printf("ingest: skipping enrichment for %q (no raw text)", art.ID)
				enriched[idx] = art
				return
			}

			// Run NER and embedding in parallel for each article.
			var innerWg sync.WaitGroup
			var entities []article.Entity
			var embedding []float64

			if hasML {
				innerWg.Add(1)
				go func() {
					defer innerWg.Done()
					mlStart := time.Now()
					ents, err := h.mlClient.Analyze(art.ID, art.RawText)
					h.mlInferTimings.Add(time.Since(mlStart))
					atomic.AddInt64(&h.mlAttempts, 1)

					if err != nil {
						log.Printf("ingest: NER failed for %q: %v", art.ID, err)
						return
					}
					atomic.AddInt64(&h.mlSuccesses, 1)
					entities = ents
				}()
			}

			if hasEmbed {
				innerWg.Add(1)
				go func() {
					defer innerWg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
					defer cancel()

					emb, err := h.embedClient.Embed(ctx, art.RawText)
					if err != nil {
						log.Printf("ingest: embedding failed for %q: %v", art.ID, err)
						return
					}
					embedding = emb
				}()
			}

			innerWg.Wait()

			art.Entities = entities
			art.Embedding = embedding
			enriched[idx] = art

			log.Printf("ingest: enriched %q — %d entities, %d-dim embedding",
				art.ID, len(entities), len(embedding))
		}(i, a)
	}

	wg.Wait()

	return enriched
}
