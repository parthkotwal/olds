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
	embedClient   *embedclient.Client
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

	// hydrationReady is closed once startup hydration from Postgres has
	// completed or failed. Article reads wait on it briefly so a cold backend
	// does not serve an empty feed while the in-memory store is still loading.
	hydrationReady chan struct{}
	hydrationOnce  sync.Once

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
	traversalTimings   timing.Buffer // graph Neighbors() call duration
	wsPushTimings      timing.Buffer // WebSocket "connections" message write duration
	mlInferTimings     timing.Buffer // ML service Analyze() call duration
	llmExplainTimings  timing.Buffer // LLM Explain() call duration
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
		hydrationReady:   make(chan struct{}),
	}
}

// MarkHydrated releases article reads that were waiting for startup hydration.
// It is safe to call more than once.
func (h *ArticleHandler) MarkHydrated() {
	h.hydrationOnce.Do(func() {
		close(h.hydrationReady)
	})
}

func (h *ArticleHandler) waitForHydration(c *gin.Context) bool {
	select {
	case <-h.hydrationReady:
		return true
	default:
	}

	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()

	select {
	case <-h.hydrationReady:
		return true
	case <-timer.C:
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "article store is warming up",
		})
		return false
	case <-c.Request.Context().Done():
		return false
	}
}

func (h *ArticleHandler) waitForArticleStore(c *gin.Context) bool {
	if h.store.Count() > 0 {
		return true
	}

	return h.waitForHydration(c)
}

func (h *ArticleHandler) isHydrated() bool {
	select {
	case <-h.hydrationReady:
		return true
	default:
		return false
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

		go func(idx int, c Connection) {
			defer wg.Done()

			if exp, ok := h.explanationCache.Get(source.ID, c.Article.ID); ok {
				connections[idx].Explanation = exp
				return
			}

			// Look up the full article from the store for entity comparison.
			fullNeighbour, ok := h.store.GetByID(c.Article.ID)
			if !ok {
				return
			}

			shared := graph.SharedEntities(source, fullNeighbour)

			exp, err := h.llmClient.Explain(
				source.Title, source.Description, source.Category,
				c.Article.Title, c.Article.Description, c.Article.Category,
				shared, c.Weight,
			)
			if err != nil {
				log.Printf("llm: explain failed for %s↔%s: %v", source.ID, c.Article.ID, err)
				return
			}

			h.explanationCache.Set(source.ID, c.Article.ID, exp)
			connections[idx].Explanation = exp
		}(i, conn)
	}

	wg.Wait()
	return connections
}

// ArticleSummary is the slim JSON shape sent to the frontend for feed listings
// and connection sidebar entries. It omits raw_text, entities, and embedding
// which are only needed internally by the graph and ML pipeline.
type ArticleSummary struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	Source      string    `json:"source"`
	Category    string    `json:"category"`
	PublishedAt time.Time `json:"published_at"`
	ImageURL    string    `json:"image_url,omitempty"`
}

func toSummary(a article.Article) ArticleSummary {
	return ArticleSummary{
		ID:          a.ID,
		Title:       a.Title,
		Description: a.Description,
		URL:         a.URL,
		Source:      a.Source,
		Category:    a.Category,
		PublishedAt: a.PublishedAt,
		ImageURL:    a.ImageURL,
	}
}

func toSummaries(articles []article.Article) []ArticleSummary {
	out := make([]ArticleSummary, len(articles))
	for i, a := range articles {
		out[i] = toSummary(a)
	}
	return out
}

// ArticleDetail is the full article shape returned by GET /articles/:id.
// Includes raw_text and entities for the reading view, but still omits
// the embedding vector which is only needed internally.
type ArticleDetail struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	URL         string           `json:"url"`
	Source      string           `json:"source"`
	Category    string           `json:"category"`
	PublishedAt time.Time        `json:"published_at"`
	ImageURL    string           `json:"image_url,omitempty"`
	RawText     string           `json:"raw_text,omitempty"`
	Entities    []article.Entity `json:"entities,omitempty"`
}

func toDetail(a article.Article) ArticleDetail {
	return ArticleDetail{
		ID:          a.ID,
		Title:       a.Title,
		Description: a.Description,
		URL:         a.URL,
		Source:      a.Source,
		Category:    a.Category,
		PublishedAt: a.PublishedAt,
		ImageURL:    a.ImageURL,
		RawText:     a.RawText,
		Entities:    a.Entities,
	}
}

// GetByID handles GET /articles/:id — returns a single article with full detail.
func (h *ArticleHandler) GetByID(c *gin.Context) {
	if !h.waitForArticleStore(c) {
		return
	}

	id := c.Param("id")
	a, ok := h.store.GetByID(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "article not found", "id": id})
		return
	}
	c.JSON(http.StatusOK, toDetail(a))
}

const defaultPageSize = 30

// List handles GET /articles.
// Supports pagination via ?page= (1-indexed, default 1) and ?page_size= (default 30).
// Optionally filters by ?category=.
func (h *ArticleHandler) List(c *gin.Context) {
	if !h.waitForArticleStore(c) {
		return
	}

	category := c.Query("category")

	var articles []article.Article
	if category != "" {
		articles = h.store.GetByCategory(category)
	} else {
		articles = h.store.GetAll()
	}

	if articles == nil {
		articles = []article.Article{}
	}

	articles = h.behaviorStore.ScoreAndSort(articles)

	total := len(articles)
	page := parseIntQuery(c, "page", 1)
	pageSize := parseIntQuery(c, "page_size", defaultPageSize)
	if page < 1 {
		page = 1
	}

	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	c.JSON(http.StatusOK, gin.H{
		"articles":  toSummaries(articles[start:end]),
		"count":     end - start,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
