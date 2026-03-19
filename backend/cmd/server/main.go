// Package main is the entry point for the Olds backend server.
// This file is the "composition root" — the one place in the application
// where concrete types are constructed and wired together. Everything else
// receives its dependencies through constructors, never by reaching for
// global state. This makes each package independently testable.
package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/olds/backend/internal/article"
	"github.com/olds/backend/internal/graph"
	"github.com/olds/backend/internal/handler"
	"github.com/olds/backend/internal/mlclient"
	"github.com/olds/backend/internal/newsapi"
)

func main() {
	// ── 1. Read and validate required configuration ───────────────────────────
	// Fail fast if the API key is missing. It is better to crash at startup
	// with a clear message than to start serving requests and fail silently
	// on the first ingestion attempt.
	apiKey := os.Getenv("NEWSAPI_KEY")
	if apiKey == "" {
		log.Fatal("NEWSAPI_KEY environment variable is required")
	}

	// ── 2. Construct dependencies in order ───────────────────────────────────
	// Build the dependency graph bottom-up: store and newsapi client have no
	// dependencies on each other, so either order works. The handler
	// depends on all three, so it is constructed last.
	store := article.NewStore()
	client := newsapi.NewClient(apiKey)

	// ML client is optional — if ML_SERVICE_URL is unset the backend runs
	// without enrichment (Phase 1/2 compatibility). When the full stack is
	// running via docker-compose, ML_SERVICE_URL is always set.
	var mlClient *mlclient.Client
	if mlURL := os.Getenv("ML_SERVICE_URL"); mlURL != "" {
		mlClient = mlclient.NewClient(mlURL)
		log.Printf("ML service configured at %s", mlURL)
	} else {
		log.Println("ML_SERVICE_URL not set — article enrichment disabled")
	}

	// The graph is always constructed — it starts empty and fills as articles
	// are ingested. It is separate from the Store: the Store is the source of
	// truth for article data; the Graph owns the connection topology.
	g := graph.NewGraph()

	articleHandler := handler.NewArticleHandler(store, client, mlClient, g)

	// ── 3. Register routes ───────────────────────────────────────────────────
	r := gin.Default()

	r.GET("/health", handler.Health)
	r.GET("/articles", articleHandler.List)
	r.POST("/ingest", articleHandler.Ingest)

	// ── 4. Start background ingestion goroutine ───────────────────────────────
	// `go func() { ... }()` launches an anonymous function as a goroutine —
	// a lightweight concurrent execution unit managed by the Go runtime.
	//
	// Why not just call RunStartupIngest() here directly (before r.Run)?
	// r.Run() blocks forever — it IS the server. If ingestion runs before it,
	// the server is not listening during those 4 HTTP round-trips to NewsAPI
	// and the subsequent ML calls. Launching a goroutine means the server
	// starts immediately and answers requests (returning [] initially) while
	// ingestion + enrichment happen in parallel.
	//
	// articleHandler is a pointer — the goroutine captures it by reference.
	// That is safe: the handler is fully constructed, never reassigned, and
	// its internal store uses a RWMutex for concurrent access.
	go func() {
		log.Println("starting initial article ingestion...")
		if err := articleHandler.RunStartupIngest(); err != nil {
			log.Printf("initial ingestion failed: %v", err)
			// Log and return — the server stays up with an empty store.
			// Hit POST /ingest to retry once the issue is resolved.
		}
	}()

	// ── 5. Start the HTTP server ──────────────────────────────────────────────
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server failed to start: %v", err)
	}
}
