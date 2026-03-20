package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/olds/backend/internal/article"
	"github.com/olds/backend/internal/graph"
	"github.com/olds/backend/internal/guardian"
	"github.com/olds/backend/internal/mlclient"
	"github.com/olds/backend/internal/newsapi"
)

// ArticleHandler holds the dependencies needed by article-related route handlers.
//
// This is the "handler struct" pattern — the Go idiomatic way to do dependency
// injection for HTTP handlers. Instead of reading from global variables (bad:
// hard to test, hidden coupling), each handler receives its dependencies
// explicitly through the struct.
//
// Compare to Python: this is like a Flask class-based view where you inject
// the store and client via __init__. The difference is Go has no classes —
// just a struct with methods. The methods (List, Ingest, Connections) become
// the route handlers via their pointer receiver (h *ArticleHandler).
//
// Testing benefit: in tests you can create an ArticleHandler with a
// pre-populated *article.Store full of fixture data and call List() directly —
// no HTTP, no NewsAPI, no Docker needed.
type ArticleHandler struct {
	store  *article.Store
	client *newsapi.Client
	// guardianClient may be nil if GUARDIAN_KEY is not set — the handler
	// degrades gracefully: only NewsAPI articles are ingested.
	guardianClient *guardian.Client
	// mlClient may be nil if ML_SERVICE_URL is not set — the handler
	// degrades gracefully: articles are stored without entities/embedding.
	mlClient *mlclient.Client
	graph    *graph.Graph
}

// NewArticleHandler constructs a handler with its dependencies injected.
// guardianClient and mlClient may be nil — both degrade gracefully when absent.
// This is called once in main.go — the handler is created, routes are
// registered, and then the server runs.
func NewArticleHandler(
	store *article.Store,
	client *newsapi.Client,
	guardianClient *guardian.Client,
	mlClient *mlclient.Client,
	g *graph.Graph,
) *ArticleHandler {
	return &ArticleHandler{
		store:          store,
		client:         client,
		guardianClient: guardianClient,
		mlClient:       mlClient,
		graph:          g,
	}
}

// List handles GET /articles.
// Optionally filters by the ?category= query parameter.
func (h *ArticleHandler) List(c *gin.Context) {
	// c.Query reads a URL query parameter by name.
	// Returns "" if the parameter is absent — no error needed, absence means "all".
	category := c.Query("category")

	var articles []article.Article
	if category != "" {
		articles = h.store.GetByCategory(category)
	} else {
		articles = h.store.GetAll()
	}

	// Guard against nil slice → JSON null.
	// The store returns nil when a category matches nothing. We always want
	// to return [] so the frontend gets a consistent type, never null.
	if articles == nil {
		articles = []article.Article{}
	}

	c.JSON(http.StatusOK, gin.H{
		"articles": articles,
		"count":    len(articles),
	})
}
