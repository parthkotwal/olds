package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/olds/backend/internal/article"
)

// defaultTopN is the number of connections returned when the caller does not
// specify ?top_n=. 10 fits comfortably in the frontend sidebar (Phase 7).
const defaultTopN = 10

// defaultMinWeight is the minimum edge weight below which a neighbour is not
// returned. 0.1 filters out near-zero connections while keeping weak-but-real
// ones. The frontend slider (Phase 7) can tighten this further at query time.
const defaultMinWeight = 0.1

// Connection is the per-neighbour element in the /connections response.
// It bundles the full Article with its edge weight, a cross-topic flag, and
// an LLM-generated explanation of why the two articles are connected.
//
// Explanation is populated by enrichWithExplanations() in articles.go.
// omitempty means it is omitted from JSON when empty — connections without
// explanations (e.g. when LLM_API_KEY is unset) are still valid.
type Connection struct {
	Article     article.Article `json:"article"`
	Weight      float64         `json:"weight"`
	CrossTopic  bool            `json:"cross_topic"`
	Explanation string          `json:"explanation,omitempty"`
}

// ConnectionsResponse is the full JSON body returned by GET /articles/:id/connections.
type ConnectionsResponse struct {
	Source      article.Article `json:"source"`
	Connections []Connection    `json:"connections"`
	Count       int             `json:"count"`
}

// Connections handles GET /articles/:id/connections.
//
// It traverses the article graph to find related articles, optionally filtered
// to cross-topic neighbours only. This is the core feature of Olds.
//
// Path parameter:
//
//	:id — the article ID to find connections for
//
// Query parameters (all optional):
//
//	top_n      int     default 10   — maximum number of connections to return
//	min_weight float64 default 0.1  — minimum edge weight (0.0–1.0)
//	cross_topic bool   default false — if true, only return articles from a
//	                                   different category than the source
//
// Example:
//
//	GET /articles/a3f9c1d2/connections?top_n=5&min_weight=0.3&cross_topic=true
func (h *ArticleHandler) Connections(c *gin.Context) {
	// ── 1. Read and validate path parameter ──────────────────────────────────
	// c.Param reads a named segment from the URL path.
	// For route GET /articles/:id/connections, c.Param("id") is the article ID.
	id := c.Param("id")

	// ── 2. Look up the source article ─────────────────────────────────────────
	// The store is the source of truth for article data (the graph stores a
	// copy for edge computation, but we always serve data from the store).
	sourceArticle, ok := h.store.GetByID(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "article not found",
			"id":    id,
		})
		return
	}

	// ── 3. Parse query parameters ─────────────────────────────────────────────
	topN := parseIntQuery(c, "top_n", defaultTopN)
	minWeight := parseFloatQuery(c, "min_weight", defaultMinWeight)
	crossTopicOnly := parseBoolQuery(c, "cross_topic", false)

	// ── 4. Traverse the graph ─────────────────────────────────────────────────
	// graph.Neighbors returns edges sorted by weight descending, filtered by
	// minWeight, limited to topN. It returns []Edge{} (never nil) if the
	// article has no neighbours above the threshold.
	edges := h.graph.Neighbors(id, topN, minWeight)

	// ── 5. Hydrate edges → full Connection objects ────────────────────────────
	// The graph stores only article IDs and weights. We look up the full
	// Article from the store for each neighbour. Articles that were evicted
	// from the store (shouldn't happen, but defensive) are silently skipped.
	connections := make([]Connection, 0, len(edges))
	for _, edge := range edges {
		neighbourArticle, found := h.store.GetByID(edge.ArticleID)
		if !found {
			// Graph and store are slightly out of sync — possible during a
			// concurrent re-ingestion. Skip rather than returning a partial object.
			continue
		}

		isCrossTopic := neighbourArticle.Category != sourceArticle.Category

		// If the caller asked for cross-topic only, skip same-category neighbours.
		if crossTopicOnly && !isCrossTopic {
			continue
		}

		connections = append(connections, Connection{
			Article:    neighbourArticle,
			Weight:     edge.Weight,
			CrossTopic: isCrossTopic,
		})
	}

	// Enrich each connection with an LLM-generated explanation.
	// No-op if LLM_API_KEY is not set.
	connections = h.enrichWithExplanations(sourceArticle, connections)

	c.JSON(http.StatusOK, ConnectionsResponse{
		Source:      sourceArticle,
		Connections: connections,
		Count:       len(connections),
	})
}

// ── Query parameter helpers ───────────────────────────────────────────────────
// These small helpers keep Connections() readable by moving the
// parse-or-default logic out of the main flow.
//
// In Python you would write: int(request.args.get("top_n", 10))
// In Go there is no built-in query → int conversion in Gin, so we do it
// explicitly. strconv.Atoi converts a string to int; it returns (int, error).
// If the parse fails (absent param or non-numeric), we return the default.

func parseIntQuery(c *gin.Context, key string, defaultVal int) int {
	raw := c.Query(key)
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return defaultVal
	}
	return v
}

func parseFloatQuery(c *gin.Context, key string, defaultVal float64) float64 {
	raw := c.Query(key)
	if raw == "" {
		return defaultVal
	}
	// strconv.ParseFloat(s, bitSize): bitSize=64 means parse as float64.
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 || v > 1 {
		return defaultVal
	}
	return v
}

func parseBoolQuery(c *gin.Context, key string, defaultVal bool) bool {
	raw := c.Query(key)
	if raw == "" {
		return defaultVal
	}
	// strconv.ParseBool accepts "1", "t", "true", "TRUE", "0", "f", "false".
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return defaultVal
	}
	return v
}
