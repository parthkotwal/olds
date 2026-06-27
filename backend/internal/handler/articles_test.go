package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/olds/backend/internal/article"
	"github.com/olds/backend/internal/behavior"
	"github.com/olds/backend/internal/graph"
	"github.com/olds/backend/internal/repository"
)

type fakeBehaviorRepo struct{}

func (fakeBehaviorRepo) RecordEvent(context.Context, behavior.Event) error {
	return nil
}

func (fakeBehaviorRepo) LoadSignals(context.Context) (map[string]behavior.ArticleSignals, error) {
	return map[string]behavior.ArticleSignals{}, nil
}

type fakeSnapshotRepo struct{}

func (fakeSnapshotRepo) Save(context.Context, repository.Snapshot) error {
	return nil
}

func (fakeSnapshotRepo) LoadRecent(context.Context, int) ([]repository.Snapshot, error) {
	return []repository.Snapshot{}, nil
}

func newTestHandler(t *testing.T) *ArticleHandler {
	t.Helper()

	store := article.NewStore()
	behaviorStore := behavior.NewStore()
	g := graph.NewGraph()
	now := time.Now()

	articles := []article.Article{
		{
			ID:          "fresh-tech",
			Title:       "Fresh tech story",
			Description: "Fresh tech description",
			URL:         "https://example.com/fresh-tech",
			Source:      "Example",
			Category:    "technology",
			PublishedAt: now.Add(-1 * time.Hour),
			Entities:    []article.Entity{{Text: "OpenAI", Label: "ORG"}},
			Embedding:   []float64{1, 0, 0},
		},
		{
			ID:          "older-world",
			Title:       "Older world story",
			Description: "Older world description",
			URL:         "https://example.com/older-world",
			Source:      "Example",
			Category:    "general",
			PublishedAt: now.Add(-20 * time.Hour),
			Entities:    []article.Entity{{Text: "NATO", Label: "ORG"}},
			Embedding:   []float64{0, 1, 0},
		},
		{
			ID:          "related-world",
			Title:       "Related world story",
			Description: "Related world description",
			URL:         "https://example.com/related-world",
			Source:      "Example",
			Category:    "general",
			PublishedAt: now.Add(-10 * time.Hour),
			Entities:    []article.Entity{{Text: "NATO", Label: "ORG"}},
			Embedding:   []float64{0, 1, 0},
		},
	}

	store.Add(articles)
	g.Add(articles)
	behaviorStore.Record(behavior.Event{ArticleID: "older-world", Type: behavior.EventDwell, Value: 120})

	h := NewArticleHandler(
		store,
		nil,
		nil,
		nil,
		nil,
		g,
		behaviorStore,
		nil,
		fakeBehaviorRepo{},
		fakeSnapshotRepo{},
		nil,
	)
	h.MarkHydrated()
	return h
}

func TestListReturnsRankedArticleSummaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestHandler(t)

	router := gin.New()
	router.GET("/articles", h.List)

	req := httptest.NewRequest(http.MethodGet, "/articles?page=1&page_size=2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body struct {
		Articles []ArticleSummary `json:"articles"`
		Total    int              `json:"total"`
		Count    int              `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Total != 3 || body.Count != 2 {
		t.Fatalf("unexpected pagination metadata: total=%d count=%d", body.Total, body.Count)
	}
	if len(body.Articles) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(body.Articles))
	}
	if body.Articles[0].Category != "general" {
		t.Fatalf("expected behavior-ranked general article first, got %q", body.Articles[0].Category)
	}
}

func TestGetByIDReturnsArticleDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestHandler(t)

	router := gin.New()
	router.GET("/articles/:id", h.GetByID)

	req := httptest.NewRequest(http.MethodGet, "/articles/older-world", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body ArticleDetail
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ID != "older-world" {
		t.Fatalf("expected older-world detail, got %q", body.ID)
	}
	if len(body.Entities) != 1 || body.Entities[0].Text != "NATO" {
		t.Fatalf("expected detail entities, got %#v", body.Entities)
	}
}

func TestMetricsReturnsPublicSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestHandler(t)

	router := gin.New()
	router.GET("/metrics", h.Metrics)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body struct {
		ArticlesIndexed int `json:"articles_indexed"`
		Graph           struct {
			Nodes       int `json:"nodes"`
			UniqueEdges int `json:"unique_edges"`
		} `json:"graph"`
		LatencyMS map[string]map[string]int64 `json:"latency_ms"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ArticlesIndexed != 3 || body.Graph.Nodes != 3 {
		t.Fatalf("unexpected metrics counts: articles=%d nodes=%d", body.ArticlesIndexed, body.Graph.Nodes)
	}
	if _, ok := body.LatencyMS["graph_traversal"]; !ok {
		t.Fatal("expected graph_traversal latency summary")
	}
}

func TestConnectionsReturnWhyConnectedBreakdown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestHandler(t)

	router := gin.New()
	router.GET("/articles/:id/connections", h.Connections)

	req := httptest.NewRequest(http.MethodGet, "/articles/older-world/connections?top_n=2&min_weight=0.1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body ConnectionsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Connections) == 0 {
		t.Fatal("expected at least one connection")
	}

	conn := body.Connections[0]
	if conn.Breakdown.Weight <= 0 {
		t.Fatalf("expected positive breakdown weight, got %.3f", conn.Breakdown.Weight)
	}
	if len(conn.Breakdown.SharedEntities) == 0 {
		t.Fatalf("expected shared entities in breakdown, got %#v", conn.Breakdown.SharedEntities)
	}
	if conn.Breakdown.SemanticPct+conn.Breakdown.EntityPct < 99.9 {
		t.Fatalf("expected contribution percentages near 100, got semantic=%.2f entity=%.2f",
			conn.Breakdown.SemanticPct, conn.Breakdown.EntityPct)
	}
}
