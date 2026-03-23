package handler

// stats.go provides two endpoints for Phase 14 stress-test observability:
//
//   GET /stats         — current system state (point-in-time snapshot)
//   GET /stats/history — time series of past snapshots from Postgres
//
// Neither endpoint is auth-protected — they expose no user data.

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Stats returns a JSON snapshot of the system's current state.
//
// Response shape:
//
//	{
//	  "graph": {
//	    "node_count": 312,
//	    "directed_edges": 4840,
//	    "unique_edges": 2420,
//	    "avg_edges_per_node": 15.5,
//	    "isolated_nodes": 3,
//	    "max_edges_per_node": 48,
//	    "density_pct": 4.97
//	  },
//	  "articles": {
//	    "total": 312,
//	    "by_category": { "general": 120, "technology": 80, ... },
//	    "decay": { "fresh": 45, "recent": 130, "aging": 90, "stale": 47 }
//	  },
//	  "ingestion": {
//	    "run_count": 14,
//	    "last_run_at": "2026-03-21T01:30:00Z",
//	    "last_run_articles": 84
//	  }
//	}
//
// Key stress-test signals to watch:
//   - density_pct growing faster than expected → edge threshold too permissive
//   - isolated_nodes increasing → ML enrichment failing for new articles
//   - decay.stale high relative to fresh+recent → ingestion interval too slow
//     or decay window too aggressive
//   - avg_edges_per_node > ~30 → graph getting noisy, traversal may slow down
func (h *ArticleHandler) Stats(c *gin.Context) {
	graphStats := h.graph.Stats()
	categoryBreakdown := h.store.CountByCategory()
	decaySnapshot := h.store.DecaySnapshot()

	// Read ingestion telemetry under lock — updated by the ingestion goroutine.
	h.ingestMu.Lock()
	runCount := h.ingestRunCount
	lastAt := h.lastIngestAt
	lastCount := h.lastIngestCount
	h.ingestMu.Unlock()

	// Format lastAt as RFC3339 string, or "never" if ingestion hasn't completed yet.
	lastAtStr := "never"
	if !lastAt.IsZero() {
		lastAtStr = lastAt.UTC().Format("2006-01-02T15:04:05Z")
	}

	c.JSON(http.StatusOK, gin.H{
		"graph": graphStats,
		"articles": gin.H{
			"total":       h.store.Count(),
			"by_category": categoryBreakdown,
			"decay":       decaySnapshot,
		},
		"ingestion": gin.H{
			"run_count":         runCount,
			"last_run_at":       lastAtStr,
			"last_run_articles": lastCount,
		},
	})
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
