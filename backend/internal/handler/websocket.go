package handler

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/olds/backend/internal/graph"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type explanationUpdate struct {
	ArticleID   string `json:"article_id"`
	Explanation string `json:"explanation"`
}

// WSConnections handles GET /ws/connections/:id.
func (h *ArticleHandler) WSConnections(c *gin.Context) {
	id := c.Param("id")

	sourceArticle, ok := h.store.GetByID(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "article not found", "id": id})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("ws: upgrade failed for article %s: %v", id, err)
		return
	}
	defer conn.Close()

	log.Printf("ws: client connected for article %s", id)

	traversalStart := time.Now()
	edges := h.graph.Neighbors(id, 10, 0.1)
	traversalElapsed := time.Since(traversalStart)
	h.traversalTimings.Add(traversalElapsed)

	if traversalElapsed > 50*time.Millisecond {
		log.Printf("ws: SLOW traversal for article %s: %v (%d nodes)", id, traversalElapsed, h.graph.NodeCount())
	} else {
		log.Printf("ws: traversal for article %s: %v (%d nodes)", id, traversalElapsed, h.graph.NodeCount())
	}

	connections := make([]Connection, 0, len(edges))
	for _, edge := range edges {
		neighbour, found := h.store.GetByID(edge.ArticleID)
		if !found {
			continue
		}
		connections = append(connections, Connection{
			Article:    toSummary(neighbour),
			Weight:     edge.Weight,
			CrossTopic: neighbour.Category != sourceArticle.Category,
		})
	}

	// Send connections immediately — slim payloads, no LLM wait.
	pushStart := time.Now()
	if err := conn.WriteJSON(WSMessage{
		Type: "connections",
		Data: ConnectionsResponse{
			Source:      toSummary(sourceArticle),
			Connections: connections,
			Count:       len(connections),
		},
	}); err != nil {
		log.Printf("ws: write failed for article %s: %v", id, err)
		return
	}
	h.wsPushTimings.Add(time.Since(pushStart))
	log.Printf("ws: pushed %d connections for article %s — streaming explanations", len(connections), id)

	// Stream LLM explanations as they resolve.
	if h.llmClient != nil && len(connections) > 0 {
		updates := make(chan explanationUpdate, len(connections))

		var wg sync.WaitGroup
		for _, item := range connections {
			wg.Add(1)

			go func(connItem Connection) {
				defer wg.Done()

				if exp, ok := h.explanationCache.Get(sourceArticle.ID, connItem.Article.ID); ok {
					if exp != "" {
						updates <- explanationUpdate{ArticleID: connItem.Article.ID, Explanation: exp}
					}
					return
				}

				fullNeighbour, ok := h.store.GetByID(connItem.Article.ID)
				if !ok {
					return
				}
				shared := graph.SharedEntities(sourceArticle, fullNeighbour)
				llmStart := time.Now()
				exp, err := h.llmClient.Explain(
					sourceArticle.Title, sourceArticle.Description, sourceArticle.Category,
					connItem.Article.Title, connItem.Article.Description, connItem.Article.Category,
					shared, connItem.Weight,
				)
				if err != nil {
					log.Printf("ws: llm explain failed for %s<->%s: %v", id, connItem.Article.ID, err)
					return
				}
				h.llmExplainTimings.Add(time.Since(llmStart))

				h.explanationCache.Set(sourceArticle.ID, connItem.Article.ID, exp)
				log.Printf("ws: explanation ready for %s<->%s", id, connItem.Article.ID)
				updates <- explanationUpdate{ArticleID: connItem.Article.ID, Explanation: exp}
			}(item)
		}

		go func() {
			wg.Wait()
			close(updates)
		}()

		for update := range updates {
			if err := conn.WriteJSON(WSMessage{Type: "explanation", Data: update}); err != nil {
				log.Printf("ws: explanation write failed for article %s: %v", id, err)
				for range updates {
				}
				return
			}
		}
	}

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			log.Printf("ws: client disconnected from article %s", id)
			break
		}
	}
}
