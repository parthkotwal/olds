package handler

// stats.go — Phase 17 metrics collection endpoint.
//
// GET /stats  — current system state including performance percentiles,
//               ingestion pipeline health, and connection quality stats.
//               JWT-protected (see main.go route registration).
//
// GET /stats/history — time series of past snapshots from Postgres.
//                      JWT-protected.

import (
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/olds/backend/internal/timing"
)

// Stats returns a comprehensive JSON snapshot of system metrics.
//
// Sections:
//
//	graph       — topology (nodes, edges, density, cross-topic ratio)
//	articles    — total count, category breakdown, decay tiers
//	ingestion   — run count, last run, ML success rate, articles/day (7-day avg)
//	performance — p50/p95 for graph traversal, WS push, ML inference, LLM explain,
//	              and full ingest cycle (rolling window of last 1,000 ops each)
//	connection_quality — type distribution, top 10 entities, noisy label stats
func (h *ArticleHandler) Stats(c *gin.Context) {
	// ── Graph topology ────────────────────────────────────────────────────────
	graphStats := h.graph.Stats()
	connTypes := h.graph.ConnTypes()
	topEntities := h.graph.TopEntities(10)
	noisyLabels := h.graph.NoisyLabels()

	// ── Article store ─────────────────────────────────────────────────────────
	categoryBreakdown := h.store.CountByCategory()
	decaySnapshot := h.store.DecaySnapshot()

	// ── Ingestion telemetry (guarded by ingestMu) ─────────────────────────────
	h.ingestMu.Lock()
	runCount := h.ingestRunCount
	lastAt := h.lastIngestAt
	lastCount := h.lastIngestCount
	// Copy history slice so we can compute the 7-day average outside the lock.
	historyCopy := make([]ingestRun, len(h.ingestHistory))
	copy(historyCopy, h.ingestHistory)
	h.ingestMu.Unlock()

	lastAtStr := "never"
	if !lastAt.IsZero() {
		lastAtStr = lastAt.UTC().Format("2006-01-02T15:04:05Z")
	}

	// ── ML enrichment counters (atomic — no lock needed) ─────────────────────
	mlAttempts := atomic.LoadInt64(&h.mlAttempts)
	mlSuccesses := atomic.LoadInt64(&h.mlSuccesses)
	var enrichmentSuccessRatePct float64
	if mlAttempts > 0 {
		enrichmentSuccessRatePct = float64(mlSuccesses) / float64(mlAttempts) * 100
	}

	// ── 7-day rolling average (articles ingested per day) ────────────────────
	// Sum article counts from runs within the last 7 days, then divide by 7.
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	var recentArticleCount int
	for _, run := range historyCopy {
		if run.at.After(sevenDaysAgo) {
			recentArticleCount += run.count
		}
	}
	articlesPerDay7d := float64(recentArticleCount) / 7.0

	// ── Performance percentiles (from ring buffers) ───────────────────────────
	// Each Percentile() call copies the current window and sorts — O(N log N)
	// where N ≤ 1,000. Acceptable for an on-demand endpoint.
	perfSection := gin.H{
		"graph_traversal_ms": timingSummary(&h.traversalTimings),
		"ws_push_ms":         timingSummary(&h.wsPushTimings),
		"ml_infer_ms":        timingSummary(&h.mlInferTimings),
		"llm_explain_ms":     timingSummary(&h.llmExplainTimings),
		"ingest_total_ms":    timingSummary(&h.ingestTotalTimings),
	}

	c.JSON(http.StatusOK, gin.H{
		"graph": graphStats,
		"articles": gin.H{
			"total":       h.store.Count(),
			"by_category": categoryBreakdown,
			"decay":       decaySnapshot,
		},
		"ingestion": gin.H{
			"run_count":                   runCount,
			"last_run_at":                 lastAtStr,
			"last_run_articles":           lastCount,
			"total_ml_attempts":           mlAttempts,
			"total_ml_successes":          mlSuccesses,
			"enrichment_success_rate_pct": enrichmentSuccessRatePct,
			"articles_per_day_7d":         articlesPerDay7d,
		},
		"performance": perfSection,
		"connection_quality": gin.H{
			"type_distribution":      connTypes,
			"top_entities":           topEntities,
			"noisy_labels":           noisyLabels,
			"explanation_cache_size": h.explanationCache.Size(),
		},
	})
}

// Metrics returns a compact public metrics snapshot for demos, README numbers,
// and lightweight uptime checks. Detailed history remains behind /stats.
func (h *ArticleHandler) Metrics(c *gin.Context) {
	graphStats := h.graph.Stats()
	mlAttempts := atomic.LoadInt64(&h.mlAttempts)
	mlSuccesses := atomic.LoadInt64(&h.mlSuccesses)
	h.ingestMu.Lock()
	runCount := h.ingestRunCount
	lastCount := h.lastIngestCount
	h.ingestMu.Unlock()

	var enrichmentSuccessRatePct float64
	if mlAttempts > 0 {
		enrichmentSuccessRatePct = float64(mlSuccesses) / float64(mlAttempts) * 100
	}

	c.JSON(http.StatusOK, gin.H{
		"articles_indexed": h.store.Count(),
		"graph": gin.H{
			"nodes":                 graphStats.NodeCount,
			"unique_edges":          graphStats.UniqueEdges,
			"directed_edges":        graphStats.DirectedEdges,
			"avg_edges_per_node":    graphStats.AvgEdgesPerNode,
			"density_pct":           graphStats.DensityPct,
			"cross_topic_ratio_pct": graphStats.CrossTopicRatioPct,
		},
		"latency_ms": gin.H{
			"graph_traversal": timingSummary(&h.traversalTimings),
			"websocket_push":  timingSummary(&h.wsPushTimings),
			"ml_inference":    timingSummary(&h.mlInferTimings),
			"llm_explanation": timingSummary(&h.llmExplainTimings),
			"ingestion_total": timingSummary(&h.ingestTotalTimings),
		},
		"ingestion": gin.H{
			"run_count":                   runCount,
			"last_run_articles":           lastCount,
			"enrichment_success_rate_pct": enrichmentSuccessRatePct,
		},
		"explanation_cache_size": h.explanationCache.Size(),
	})
}

func timingSummary(b *timing.Buffer) gin.H {
	return gin.H{
		"p50":     b.Percentile(50).Milliseconds(),
		"p95":     b.Percentile(95).Milliseconds(),
		"p99":     b.Percentile(99).Milliseconds(),
		"samples": b.Count(),
	}
}

// StatsHistory handles GET /stats/history.
//
// Returns the last N snapshots (default 100, max 500) ordered newest-first.
// Each row is one ingestion run's worth of system metrics, so charting
// node_count, density_pct, and cross_topic_ratio_pct over captured_at gives
// the full stress-test growth curve.
//
// Query parameters:
//
//	limit  int  default 100 — number of snapshots to return (max 500)
func (h *ArticleHandler) StatsHistory(c *gin.Context) {
	limit := 100
	if raw := c.Query("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}
	// Cap at 500 to prevent accidentally fetching thousands of rows.
	if limit > 500 {
		limit = 500
	}

	snapshots, err := h.snapshotRepo.LoadRecent(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"snapshots": snapshots,
		"count":     len(snapshots),
	})
}
