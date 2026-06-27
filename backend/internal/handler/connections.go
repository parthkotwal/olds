package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/olds/backend/internal/article"
	"github.com/olds/backend/internal/graph"
)

const defaultTopN = 10
const defaultMinWeight = 0.1

// Connection is the per-neighbour element in connection responses.
// Uses ArticleSummary instead of the full Article to avoid sending
// raw_text, entities, and embedding vectors to the frontend.
type Connection struct {
	Article     ArticleSummary  `json:"article"`
	Weight      float64         `json:"weight"`
	CrossTopic  bool            `json:"cross_topic"`
	Breakdown   graph.Breakdown `json:"breakdown"`
	Explanation string          `json:"explanation,omitempty"`
}

type ConnectionsResponse struct {
	Source      ArticleSummary `json:"source"`
	Connections []Connection   `json:"connections"`
	Count       int            `json:"count"`
}

// Connections handles GET /articles/:id/connections.
func (h *ArticleHandler) Connections(c *gin.Context) {
	if !h.waitForArticleStore(c) {
		return
	}

	id := c.Param("id")

	sourceArticle, ok := h.store.GetByID(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "article not found",
			"id":    id,
		})
		return
	}

	topN := parseIntQuery(c, "top_n", defaultTopN)
	minWeight := parseFloatQuery(c, "min_weight", defaultMinWeight)
	crossTopicOnly := parseBoolQuery(c, "cross_topic", false)

	edges := h.connectionEdges(sourceArticle, topN, minWeight)

	connections := make([]Connection, 0, len(edges))
	for _, edge := range edges {
		neighbourArticle, found := h.store.GetByID(edge.ArticleID)
		if !found {
			continue
		}

		isCrossTopic := neighbourArticle.Category != sourceArticle.Category

		if crossTopicOnly && !isCrossTopic {
			continue
		}

		connections = append(connections, Connection{
			Article:    toSummary(neighbourArticle),
			Weight:     edge.Weight,
			CrossTopic: isCrossTopic,
			Breakdown:  graph.Explain(sourceArticle, neighbourArticle),
		})
	}

	connections = h.enrichWithExplanations(sourceArticle, connections)

	c.JSON(http.StatusOK, ConnectionsResponse{
		Source:      toSummary(sourceArticle),
		Connections: connections,
		Count:       len(connections),
	})
}

func (h *ArticleHandler) connectionEdges(sourceArticle article.Article, topN int, minWeight float64) []graph.Edge {
	if h.isHydrated() {
		return h.graph.Neighbors(sourceArticle.ID, topN, minWeight)
	}

	return graph.Related(sourceArticle, h.store.GetAll(), topN, minWeight)
}

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
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return defaultVal
	}
	return v
}
