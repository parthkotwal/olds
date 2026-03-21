package handler

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/olds/backend/internal/behavior"
)

// RecordBehavior handles POST /behavior.
//
// The frontend sends a behavioral event whenever:
//   - An article is opened (reopen)
//   - The user navigates away or closes the tab (dwell, scroll_depth)
//
// All signals are implicit — the user never rates anything explicitly.
// The backend accumulates them in the behavior.Store and uses them to
// re-rank the feed in the List handler.
//
// Request body:
//
//	{ "article_id": "abc123", "type": "dwell", "value": 47.3 }
//
// Response:
//
//	{ "ok": true }
func (h *ArticleHandler) RecordBehavior(c *gin.Context) {
	var event behavior.Event

	// ShouldBindJSON deserializes the request body into event and returns
	// an error if the JSON is malformed or required fields are missing.
	// It's the idiomatic Gin way to parse + validate a request body.
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate the event type — reject unknown types rather than silently
	// ignoring them. This catches frontend bugs early.
	switch event.Type {
	case behavior.EventDwell, behavior.EventScrollDepth, behavior.EventReopen:
		// valid — fall through
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "unknown event type",
			"type":  event.Type,
		})
		return
	}

	if event.ArticleID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "article_id is required"})
		return
	}

	h.behaviorStore.Record(event)

	// Persist to Postgres asynchronously. Behavior events are fired frequently
	// (every article open, every scroll, on tab close), so we fire-and-forget
	// the DB write to avoid adding latency to every event. Losing occasional
	// events due to a DB hiccup is acceptable — these are soft signals, not
	// financial transactions. Errors are logged for observability.
	go func() {
		if err := h.behaviorRepo.RecordEvent(context.Background(), event); err != nil {
			log.Printf("behavior: DB persist failed for article %s: %v", event.ArticleID, err)
		}
	}()

	// 200 OK with a minimal body — the frontend fires these as "best effort"
	// keepalive requests and doesn't wait for the response in most cases.
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
