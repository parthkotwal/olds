package handler

// WebSocket handler for real-time graph traversal and streaming LLM explanations.
//
// Protocol (two message types):
//
//   1. "connections" — sent immediately after graph traversal. Contains the
//      full connection list with article data and weights, but NO explanations
//      yet. The frontend renders connections right away without waiting for LLM.
//
//   2. "explanation"  — sent once per connection as each LLM call resolves.
//      Contains { article_id, explanation }. The frontend patches the matching
//      connection in-place. This streams explanations in as they're ready
//      rather than blocking until all are done.
//
// Why a channel? gorilla/websocket requires single-writer access — two
// goroutines cannot call WriteJSON concurrently or the connection will corrupt.
// A channel serializes the concurrent LLM goroutines into a single write loop.

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/olds/backend/internal/graph"
)

// upgrader converts an HTTP connection into a WebSocket connection.
// CheckOrigin returns true unconditionally — correct for development where
// the frontend (port 5173) and backend (port 8080) are on different ports.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSMessage is the typed envelope for every message sent over the WebSocket.
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// explanationUpdate is the payload for "explanation" messages.
// Sent once per connection as each LLM call resolves.
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

	// ── Graph traversal ───────────────────────────────────────────────────────
	traversalStart := time.Now()
	edges := h.graph.Neighbors(id, 10, 0.1)
	traversalElapsed := time.Since(traversalStart)
	h.traversalTimings.Add(traversalElapsed) // Phase 17: record for /stats percentiles

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
			Article:    neighbour,
			Weight:     edge.Weight,
			CrossTopic: neighbour.Category != sourceArticle.Category,
		})
	}

	// ── Step 1: send connections immediately, no LLM wait ─────────────────────
	// The frontend renders the sidebar right away. Explanations arrive separately.
	pushStart := time.Now()
	if err := conn.WriteJSON(WSMessage{
		Type: "connections",
		Data: ConnectionsResponse{
			Source:      sourceArticle,
			Connections: connections,
			Count:       len(connections),
		},
	}); err != nil {
		log.Printf("ws: write failed for article %s: %v", id, err)
		return
	}
	h.wsPushTimings.Add(time.Since(pushStart)) // Phase 17: record push latency
	log.Printf("ws: pushed %d connections for article %s — streaming explanations", len(connections), id)

	// ── Step 2: stream explanations as LLM calls resolve ─────────────────────
	// Only runs when LLM is configured and there are connections to explain.
	if h.llmClient != nil && len(connections) > 0 {
		// Buffered channel — capacity = number of connections so goroutines
		// never block on send even if the write loop is momentarily slow.
		updates := make(chan explanationUpdate, len(connections))

		var wg sync.WaitGroup
		for _, item := range connections {
			wg.Add(1)

			// Capture item as a function argument — avoids loop variable capture.
			go func(connItem Connection) {
				defer wg.Done()

				// Serve from cache when available — zero API cost, instant.
				if exp, ok := h.explanationCache.Get(sourceArticle.ID, connItem.Article.ID); ok {
					if exp != "" {
						updates <- explanationUpdate{ArticleID: connItem.Article.ID, Explanation: exp}
					}
					return
				}

				shared := graph.SharedEntities(sourceArticle, connItem.Article)
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
				h.llmExplainTimings.Add(time.Since(llmStart)) // Phase 17: record LLM latency

				h.explanationCache.Set(sourceArticle.ID, connItem.Article.ID, exp)
				log.Printf("ws: explanation ready for %s<->%s", id, connItem.Article.ID)
				updates <- explanationUpdate{ArticleID: connItem.Article.ID, Explanation: exp}
			}(item)
		}

		// Close the channel once all goroutines are done so the range loop below
		// exits cleanly. This runs in its own goroutine so it doesn't block here.
		go func() {
			wg.Wait()
			close(updates)
		}()

		// Single write loop — gorilla/websocket requires one writer at a time.
		// The channel serializes all concurrent goroutine results through here.
		for update := range updates {
			if err := conn.WriteJSON(WSMessage{Type: "explanation", Data: update}); err != nil {
				log.Printf("ws: explanation write failed for article %s: %v", id, err)
				// Drain the channel so the goroutines can exit and be GC'd.
				for range updates {
				}
				return
			}
		}
	}

	// ── Read loop — hold open until client disconnects ────────────────────────
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			log.Printf("ws: client disconnected from article %s", id)
			break
		}
	}
}
