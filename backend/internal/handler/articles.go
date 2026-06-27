package handler

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/olds/backend/internal/article"
	"github.com/olds/backend/internal/behavior"
	"github.com/olds/backend/internal/embedclient"
	"github.com/olds/backend/internal/graph"
	"github.com/olds/backend/internal/guardian"
	"github.com/olds/backend/internal/llm"
	"github.com/olds/backend/internal/mlclient"
	"github.com/olds/backend/internal/newsapi"
	"github.com/olds/backend/internal/repository"
	"github.com/olds/backend/internal/timing"
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
	// degrades gracefully: articles are stored without entities.
	mlClient *mlclient.Client
	// embedClient calls OpenAI's embeddings API. May be nil if LLM_API_KEY
	// is not set — articles are stored without embeddings.
	embedClient *embedclient.Client
	graph         *graph.Graph
	behaviorStore *behavior.Store
	// articleRepo and behaviorRepo persist data to Postgres.
	// They are never nil — always constructed in main.go.
	articleRepo  repository.ArticleRepository
	behaviorRepo repository.BehaviorRepository
	// snapshotRepo persists a system metrics row after every ingestion run.
	// Never nil — constructed in main.go for Phase 14 stress-test observability.
	snapshotRepo repository.SnapshotRepository

	// llmClient calls OpenAI to generate connection explanations.
	// May be nil if LLM_API_KEY is not set — both connection handlers degrade
	// gracefully: connections are returned without an explanation field.
	llmClient *llm.Client
	// explanationCache stores LLM-generated explanations by article pair so
	// the same pair is never sent to OpenAI more than once per process lifetime.
	explanationCache *llm.ExplanationCache

	// Ingestion telemetry — guarded by ingestMu so the /stats handler can
	// safely read these from a different goroutine than the ingestion goroutine.
	ingestMu        sync.Mutex
	ingestRunCount  int       // total number of scheduled ingestion runs completed
	lastIngestAt    time.Time // wall-clock time of the most recent completed run
	lastIngestCount int       // articles ingested in the most recent run
	// ingestHistory records per-run article counts for 7-day rolling average.
	ingestHistory []ingestRun

	// Phase 17: latency ring buffers (rolling window of last 1,000 samples each).
	// timing.Buffer has its own internal mutex — safe for concurrent access.
	traversalTimings  timing.Buffer // graph Neighbors() call duration
	wsPushTimings     timing.Buffer // WebSocket "connections" message write duration
	mlInferTimings    timing.Buffer // ML service Analyze() call duration
	llmExplainTimings timing.Buffer // LLM Explain() call duration
	ingestTotalTimings timing.Buffer // full ingest cycle: fetch → enrich → graph.Add()

	// ML enrichment success counters — updated with sync/atomic in enrich() goroutines.
	// Read with atomic.LoadInt64 in the /stats handler.
	mlAttempts  int64 // total ML Analyze() calls attempted
	mlSuccesses int64 // successful ML Analyze() calls
}

// ingestRun captures the timestamp and article count for one scheduled
// ingestion run. Used to compute the 7-day rolling average articles/day.
type ingestRun struct {
	at    time.Time
	count int
}

// NewArticleHandler constructs a handler with its dependencies injected.
// guardianClient and mlClient may be nil — both degrade gracefully when absent.
// articleRepo and behaviorRepo must not be nil.
// This is called once in main.go — the handler is created, routes are
// registered, and then the server runs.
func NewArticleHandler(
	store *article.Store,
	client *newsapi.Client,
	guardianClient *guardian.Client,
	mlClient *mlclient.Client,
	embedClient *embedclient.Client,
	g *graph.Graph,
	bs *behavior.Store,
	articleRepo repository.ArticleRepository,
	behaviorRepo repository.BehaviorRepository,
	snapshotRepo repository.SnapshotRepository,
	llmClient *llm.Client,
) *ArticleHandler {
	return &ArticleHandler{
		store:            store,
		client:           client,
		guardianClient:   guardianClient,
		mlClient:         mlClient,
		embedClient:      embedClient,
		graph:            g,
		behaviorStore:    bs,
		articleRepo:      articleRepo,
		behaviorRepo:     behaviorRepo,
		snapshotRepo:     snapshotRepo,
		llmClient:        llmClient,
		explanationCache: llm.NewExplanationCache(),
	}
}

// enrichWithExplanations populates the Explanation field on each Connection
// by calling the LLM API (or serving from cache). Called by both the REST
// connections handler and the WebSocket handler.
//
// LLM calls are made concurrently using sync.WaitGroup — the same pattern as
// ingest.go's enrich(). Each goroutine writes to its own index in the slice
// (safe without a mutex), and we block until all are done before returning.
//
// If llmClient is nil, returns the slice unchanged. If an individual call
// fails, that connection's Explanation is left empty — degrading gracefully.
func (h *ArticleHandler) enrichWithExplanations(source article.Article, connections []Connection) []Connection {
	if h.llmClient == nil {
		return connections
	}

	var wg sync.WaitGroup

	for i, conn := range connections {
		wg.Add(1)

		// Capture i and conn as function parameters to avoid the loop variable
		// capture gotcha. Each goroutine gets its own copy of i and conn.
		go func(idx int, c Connection) {
			defer wg.Done()

			// Cache hit — skip the API call entirely.
			if exp, ok := h.explanationCache.Get(source.ID, c.Article.ID); ok {
				connections[idx].Explanation = exp
				return
			}

			// Compute the shared entity set for the prompt.
			// graph.SharedEntities is a pure function — safe to call concurrently.
			shared := graph.SharedEntities(source, c.Article)

			exp, err := h.llmClient.Explain(
				source.Title, source.Description, source.Category,
				c.Article.Title, c.Article.Description, c.Article.Category,
				shared, c.Weight,
			)
			if err != nil {
				log.Printf("llm: explain failed for %s↔%s: %v", source.ID, c.Article.ID, err)
				return // leave Explanation empty — connection still shows without it
			}

			h.explanationCache.Set(source.ID, c.Article.ID, exp)
			connections[idx].Explanation = exp
			log.Printf("llm: explanation set for %s↔%s: %q", source.ID, c.Article.ID, exp)
		}(i, conn)
	}

	wg.Wait()
	return connections
}

// List handles GET /articles.
// Optionally filters by the ?category= query parameter.
// Articles are re-ranked by the behavior store before being returned —
// categories and entities the user engages with rise to the top.
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

	// Re-rank using implicit behavior signals.
	// ScoreAndSort is a no-op in terms of content — it only reorders.
	// When no signals have been recorded yet, all articles score 1.0
	// and the original ingestion order (recency) is preserved.
	articles = h.behaviorStore.ScoreAndSort(articles)

	c.JSON(http.StatusOK, gin.H{
		"articles": articles,
		"count":    len(articles),
	})
}
