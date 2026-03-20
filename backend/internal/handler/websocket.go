package handler

// WebSocket handler for real-time graph traversal.
//
// When a user opens an article, the frontend opens a WebSocket connection to
// GET /ws/connections/:id. This handler:
//   1. Upgrades the HTTP connection to a WebSocket.
//   2. Traverses the article graph to find cross-topic neighbours.
//   3. Pushes the result as a single JSON message.
//   4. Holds the connection open — the read loop detects when the user
//      navigates away (client closes the WebSocket) so we clean up.
//
// The read-loop-for-disconnect pattern is idiomatic gorilla/websocket:
// a WebSocket connection has no "done" channel built in, so you must
// ReadMessage() in a loop to notice when the peer closes the connection.

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// upgrader converts an HTTP connection into a WebSocket connection.
// CheckOrigin returns true unconditionally — this is correct for development
// where the frontend (port 5173) and backend (port 8080) are on different ports.
// In production you would check r.Header.Get("Origin") against an allow-list.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WSMessage is the envelope for every message sent over the WebSocket.
// Using a typed envelope with a "type" field lets the frontend dispatch
// on message type — useful when we add more message types in Phase 9
// (e.g. "behavior_ack", "feed_update").
//
// The Data field is interface{} (Go's "any type") because different message
// types carry different payloads. gorilla/websocket's WriteJSON encodes it
// to JSON via encoding/json — the concrete type at runtime determines the shape.
type WSMessage struct {
	Type string `json:"type"`
	// Data is encoded as whatever concrete type is stored here at write time.
	Data interface{} `json:"data"`
}

// WSConnections handles GET /ws/connections/:id.
//
// The HTTP → WebSocket upgrade is transparent to Gin's routing — the upgrade
// happens inside the handler, and from that point on the connection is a
// full-duplex WebSocket rather than a request/response pair.
func (h *ArticleHandler) WSConnections(c *gin.Context) {
	id := c.Param("id")

	// Look up the source article before upgrading. If the article doesn't
	// exist we can still return a clean HTTP 404 — once upgraded, we lose
	// the ability to send normal HTTP status codes.
	sourceArticle, ok := h.store.GetByID(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "article not found", "id": id})
		return
	}

	// Upgrade the HTTP connection to a WebSocket.
	// c.Writer and c.Request are the underlying http.ResponseWriter and *http.Request.
	// After Upgrade(), writing to c.Writer would panic — all communication
	// goes through conn from here on.
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Upgrade failure is usually a client-side issue (non-WebSocket request).
		// Gin has already written the error response; we just log and return.
		log.Printf("ws: upgrade failed for article %s: %v", id, err)
		return
	}
	// defer conn.Close() ensures the connection is cleaned up no matter how
	// this function exits — normal return, panic, or error.
	defer conn.Close()

	log.Printf("ws: client connected for article %s", id)

	// ── Graph traversal ───────────────────────────────────────────────────────
	// Traverse the graph to find the top 10 neighbours with weight ≥ 0.1.
	// This call holds the graph's RLock for the duration of the sort —
	// typically <1ms even for hundreds of articles.
	edges := h.graph.Neighbors(id, 10, 0.1)

	// Hydrate each edge into a full Connection (article data + weight + flag).
	// Reuses the Connection type defined in connections.go — same payload shape
	// as the REST endpoint, so the frontend can use one type for both.
	connections := make([]Connection, 0, len(edges))
	for _, edge := range edges {
		neighbour, found := h.store.GetByID(edge.ArticleID)
		if !found {
			// Article was removed from the store after the edge was computed.
			// Rare (in-memory store never deletes), but defensive.
			continue
		}
		connections = append(connections, Connection{
			Article:    neighbour,
			Weight:     edge.Weight,
			CrossTopic: neighbour.Category != sourceArticle.Category,
		})
	}

	// ── Push connections to the client ───────────────────────────────────────
	// WriteJSON encodes the struct to JSON and sends it as a single WebSocket
	// text frame. The frontend's ws.onmessage handler receives this.
	msg := WSMessage{
		Type: "connections",
		Data: ConnectionsResponse{
			Source:      sourceArticle,
			Connections: connections,
			Count:       len(connections),
		},
	}
	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("ws: write failed for article %s: %v", id, err)
		return
	}

	log.Printf("ws: pushed %d connections for article %s", len(connections), id)

	// ── Read loop — stay open until client disconnects ────────────────────────
	// We don't expect the client to send any messages in Phase 8 (the
	// connection is server→client only). But we must call ReadMessage() in a
	// loop to detect when the client closes the tab or navigates away.
	//
	// When the client closes the WebSocket, ReadMessage() returns an error
	// (websocket.CloseError or io.EOF). We break out, deferred conn.Close()
	// runs, and the goroutine exits — no goroutine leak.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			// Normal close (user navigated away) or unexpected disconnect.
			// Both are handled the same way: log at debug level and exit.
			log.Printf("ws: client disconnected from article %s", id)
			break
		}
	}
}
