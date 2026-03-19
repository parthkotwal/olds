// Tests for the graph package.
//
// This file uses package graph (same package, not graph_test) so it can access
// unexported functions: cosineSimilarity, entityJaccard, edgeWeight.
// This is called "white-box testing" in Go — you test internal logic directly.
// Black-box tests use package graph_test and can only call exported names.
//
// Run all tests:           go test ./internal/graph/...
// Run with race detector:  go test -race ./internal/graph/...
// Run a specific test:     go test -run TestCosineSimilarity ./internal/graph/...
package graph

import (
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/olds/backend/internal/article"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// makeArticle builds a minimal Article for testing.
func makeArticle(id, category string, embedding []float64, entities []article.Entity) article.Article {
	return article.Article{
		ID:        id,
		Category:  category,
		Embedding: embedding,
		Entities:  entities,
	}
}

// makeEntity builds an Entity with the given text and spaCy label.
func makeEntity(text, label string) article.Entity {
	return article.Entity{Text: text, Label: label}
}

// almostEqual returns true if a and b are within epsilon of each other.
// In Go (and all floating-point arithmetic), never compare floats with ==.
// Two computations that should give the same result may differ by a tiny
// rounding error. Use an epsilon comparison instead.
func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

// ── cosineSimilarity ──────────────────────────────────────────────────────────

func TestCosineSimilarity_Identical(t *testing.T) {
	v := []float64{1, 2, 3}
	got := cosineSimilarity(v, v)
	if !almostEqual(got, 1.0) {
		t.Errorf("identical vectors: got %f, want 1.0", got)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	// [1,0] and [0,1] are perpendicular — cosine similarity is 0.
	got := cosineSimilarity([]float64{1, 0}, []float64{0, 1})
	if !almostEqual(got, 0.0) {
		t.Errorf("orthogonal vectors: got %f, want 0.0", got)
	}
}

func TestCosineSimilarity_KnownValue(t *testing.T) {
	// cos([1,0], [1,1]) = 1 / (1 * sqrt(2)) ≈ 0.7071
	got := cosineSimilarity([]float64{1, 0}, []float64{1, 1})
	want := 1.0 / math.Sqrt(2)
	if !almostEqual(got, want) {
		t.Errorf("known value: got %f, want %f", got, want)
	}
}

func TestCosineSimilarity_EmptyA(t *testing.T) {
	got := cosineSimilarity([]float64{}, []float64{1, 2, 3})
	if got != 0.0 {
		t.Errorf("empty a: got %f, want 0.0", got)
	}
}

func TestCosineSimilarity_EmptyB(t *testing.T) {
	got := cosineSimilarity([]float64{1, 2, 3}, []float64{})
	if got != 0.0 {
		t.Errorf("empty b: got %f, want 0.0", got)
	}
}

func TestCosineSimilarity_BothEmpty(t *testing.T) {
	got := cosineSimilarity([]float64{}, []float64{})
	if got != 0.0 {
		t.Errorf("both empty: got %f, want 0.0", got)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	got := cosineSimilarity([]float64{0, 0, 0}, []float64{1, 2, 3})
	if got != 0.0 {
		t.Errorf("zero vector: got %f, want 0.0", got)
	}
}

func TestCosineSimilarity_MismatchedLength(t *testing.T) {
	got := cosineSimilarity([]float64{1, 2}, []float64{1, 2, 3})
	if got != 0.0 {
		t.Errorf("mismatched length: got %f, want 0.0", got)
	}
}

// ── entityJaccard ─────────────────────────────────────────────────────────────

func TestEntityJaccard_BothEmpty(t *testing.T) {
	got := entityJaccard([]article.Entity{}, []article.Entity{})
	if got != 0.0 {
		t.Errorf("both empty: got %f, want 0.0", got)
	}
}

func TestEntityJaccard_OneEmpty(t *testing.T) {
	got := entityJaccard(
		[]article.Entity{makeEntity("Xi Jinping", "PERSON")},
		[]article.Entity{},
	)
	if got != 0.0 {
		t.Errorf("one empty: got %f, want 0.0", got)
	}
}

func TestEntityJaccard_NoOverlap(t *testing.T) {
	got := entityJaccard(
		[]article.Entity{makeEntity("Xi Jinping", "PERSON")},
		[]article.Entity{makeEntity("Joe Biden", "PERSON")},
	)
	if got != 0.0 {
		t.Errorf("no overlap: got %f, want 0.0", got)
	}
}

func TestEntityJaccard_FullOverlap(t *testing.T) {
	entities := []article.Entity{makeEntity("United Nations", "ORG")}
	got := entityJaccard(entities, entities)
	if !almostEqual(got, 1.0) {
		t.Errorf("full overlap: got %f, want 1.0", got)
	}
}

func TestEntityJaccard_PartialOverlap(t *testing.T) {
	// A = {"xi jinping", "un"}
	// B = {"xi jinping", "nato"}
	// intersection = 1 ("xi jinping"), union = 3 → Jaccard = 1/3
	a := []article.Entity{
		makeEntity("Xi Jinping", "PERSON"),
		makeEntity("UN", "ORG"),
	}
	b := []article.Entity{
		makeEntity("Xi Jinping", "PERSON"),
		makeEntity("NATO", "ORG"),
	}
	got := entityJaccard(a, b)
	want := 1.0 / 3.0
	if !almostEqual(got, want) {
		t.Errorf("partial overlap: got %f, want %f", got, want)
	}
}

func TestEntityJaccard_FilteredLabels(t *testing.T) {
	// DATE entities should be ignored by the high-signal filter.
	// If both articles share only DATE entities, Jaccard should be 0.
	a := []article.Entity{makeEntity("2024", "DATE")}
	b := []article.Entity{makeEntity("2024", "DATE")}
	got := entityJaccard(a, b)
	if got != 0.0 {
		t.Errorf("filtered labels: got %f, want 0.0 (DATE should be filtered)", got)
	}
}

func TestEntityJaccard_MixedLabels(t *testing.T) {
	// "Xi Jinping" (PERSON, high-signal) + "2024" (DATE, filtered).
	// After filtering, both sets = {"xi jinping"} → Jaccard = 1.0.
	a := []article.Entity{
		makeEntity("Xi Jinping", "PERSON"),
		makeEntity("2024", "DATE"),
	}
	b := []article.Entity{
		makeEntity("Xi Jinping", "PERSON"),
	}
	got := entityJaccard(a, b)
	if !almostEqual(got, 1.0) {
		t.Errorf("mixed labels: got %f, want 1.0 (DATE filtered, only PERSON counted)", got)
	}
}

func TestEntityJaccard_CaseInsensitive(t *testing.T) {
	// "Apple" and "apple" should be treated as the same entity after lowercasing.
	a := []article.Entity{makeEntity("Apple", "ORG")}
	b := []article.Entity{makeEntity("apple", "ORG")}
	got := entityJaccard(a, b)
	if !almostEqual(got, 1.0) {
		t.Errorf("case insensitive: got %f, want 1.0", got)
	}
}

// ── edgeWeight ────────────────────────────────────────────────────────────────

func TestEdgeWeight_ZeroEmbeddingZeroEntities(t *testing.T) {
	a := makeArticle("a", "general", nil, nil)
	b := makeArticle("b", "tech", nil, nil)
	got := edgeWeight(a, b)
	if got != 0.0 {
		t.Errorf("zero inputs: got %f, want 0.0", got)
	}
}

func TestEdgeWeight_BothContribute(t *testing.T) {
	// cosine = 1.0 (identical vectors), jaccard = 1.0 (identical entities)
	// → weight = 0.6*1.0 + 0.4*1.0 = 1.0
	v := []float64{1, 0, 0}
	e := []article.Entity{makeEntity("United Nations", "ORG")}
	a := makeArticle("a", "general", v, e)
	b := makeArticle("b", "crime", v, e)
	got := edgeWeight(a, b)
	if !almostEqual(got, 1.0) {
		t.Errorf("full weight: got %f, want 1.0", got)
	}
}

func TestEdgeWeight_OnlyCosine(t *testing.T) {
	// cosine = 1.0, jaccard = 0.0 → weight = 0.6
	v := []float64{1, 0, 0}
	a := makeArticle("a", "general", v, nil)
	b := makeArticle("b", "crime", v, nil)
	got := edgeWeight(a, b)
	if !almostEqual(got, 0.6) {
		t.Errorf("only cosine: got %f, want 0.6", got)
	}
}

func TestEdgeWeight_OnlyJaccard(t *testing.T) {
	// cosine = 0.0 (empty embeddings), jaccard = 1.0 → weight = 0.4
	e := []article.Entity{makeEntity("United Nations", "ORG")}
	a := makeArticle("a", "general", nil, e)
	b := makeArticle("b", "crime", nil, e)
	got := edgeWeight(a, b)
	if !almostEqual(got, 0.4) {
		t.Errorf("only jaccard: got %f, want 0.4", got)
	}
}

// ── Graph integration ─────────────────────────────────────────────────────────

func TestGraph_EmptyNeighbors(t *testing.T) {
	g := NewGraph()
	got := g.Neighbors("nonexistent", 5, 0)
	// Must return empty slice, not nil. nil → JSON null; []Edge{} → JSON [].
	if got == nil {
		t.Error("Neighbors on empty graph: got nil, want empty slice")
	}
	if len(got) != 0 {
		t.Errorf("Neighbors on empty graph: got %d edges, want 0", len(got))
	}
}

func TestGraph_SingleArticle(t *testing.T) {
	g := NewGraph()
	a := makeArticle("a", "general", []float64{1, 0, 0}, nil)
	g.Add([]article.Article{a})

	got := g.Neighbors("a", 5, 0)
	if len(got) != 0 {
		t.Errorf("single article: got %d edges, want 0 (no self-edges)", len(got))
	}
}

func TestGraph_TwoArticlesConnected(t *testing.T) {
	g := NewGraph()
	v := []float64{1, 0, 0}
	a := makeArticle("a", "general", v, nil)
	b := makeArticle("b", "science", v, nil) // same embedding → cosine = 1.0

	g.Add([]article.Article{a, b})

	neighborsOfA := g.Neighbors("a", 5, 0)
	if len(neighborsOfA) != 1 {
		t.Fatalf("a should have 1 neighbour, got %d", len(neighborsOfA))
	}
	if neighborsOfA[0].ArticleID != "b" {
		t.Errorf("neighbour should be 'b', got %q", neighborsOfA[0].ArticleID)
	}
	if neighborsOfA[0].Weight <= 0 {
		t.Errorf("edge weight should be > 0, got %f", neighborsOfA[0].Weight)
	}

	// Edges are bidirectional — b should also have a as its neighbour.
	neighborsOfB := g.Neighbors("b", 5, 0)
	if len(neighborsOfB) != 1 || neighborsOfB[0].ArticleID != "a" {
		t.Error("edges should be bidirectional")
	}
}

func TestGraph_TopNLimiting(t *testing.T) {
	g := NewGraph()
	// Add article "hub" with a known embedding.
	hub := makeArticle("hub", "general", []float64{1, 0, 0}, nil)
	g.Add([]article.Article{hub})

	// Add 5 more articles all connected to hub with various weights.
	// Use slightly different embeddings to get different cosine values.
	for i := 1; i <= 5; i++ {
		art := makeArticle(
			fmt.Sprintf("art-%d", i),
			"science",
			[]float64{1, float64(i) * 0.01, 0}, // slightly different from hub
			nil,
		)
		g.Add([]article.Article{art})
	}

	got := g.Neighbors("hub", 3, 0)
	if len(got) != 3 {
		t.Errorf("topN=3: got %d edges, want 3", len(got))
	}

	// Verify descending sort.
	for i := 1; i < len(got); i++ {
		if got[i].Weight > got[i-1].Weight {
			t.Errorf("results not sorted descending at index %d: %f > %f",
				i, got[i].Weight, got[i-1].Weight)
		}
	}
}

func TestGraph_MinWeightFilter(t *testing.T) {
	g := NewGraph()
	// Article a: very similar embedding to hub (cosine ≈ 1.0 → weight ≈ 0.6)
	hub := makeArticle("hub", "general", []float64{1, 0, 0}, nil)
	close := makeArticle("close", "science", []float64{1, 0, 0}, nil)
	// Article far: orthogonal embedding (cosine = 0.0 → weight = 0.0, filtered)
	far := makeArticle("far", "sports", []float64{0, 1, 0}, nil)

	g.Add([]article.Article{hub, close, far})

	// minWeight = 0.5 should include "close" (weight ≈ 0.6) but exclude "far" (weight = 0.0)
	got := g.Neighbors("hub", 10, 0.5)
	if len(got) != 1 {
		t.Errorf("minWeight=0.5: got %d edges, want 1", len(got))
	}
	if len(got) == 1 && got[0].ArticleID != "close" {
		t.Errorf("expected neighbour 'close', got %q", got[0].ArticleID)
	}
}

func TestGraph_NodeCount(t *testing.T) {
	g := NewGraph()
	g.Add([]article.Article{
		makeArticle("a", "general", nil, nil),
		makeArticle("b", "sports", nil, nil),
		makeArticle("c", "science", nil, nil),
	})
	if g.NodeCount() != 3 {
		t.Errorf("NodeCount: got %d, want 3", g.NodeCount())
	}
}

func TestGraph_EdgeCountBidirectional(t *testing.T) {
	g := NewGraph()
	v := []float64{1, 0, 0}
	g.Add([]article.Article{
		makeArticle("a", "general", v, nil),
		makeArticle("b", "science", v, nil),
	})
	// One pair, two directed edges (A→B and B→A).
	if g.EdgeCount() != 2 {
		t.Errorf("EdgeCount: got %d, want 2 (bidirectional)", g.EdgeCount())
	}
}

func TestGraph_DuplicateAdd(t *testing.T) {
	g := NewGraph()
	a := makeArticle("a", "general", []float64{1, 0, 0}, nil)
	g.Add([]article.Article{a})
	g.Add([]article.Article{a}) // add same article again

	if g.NodeCount() != 1 {
		t.Errorf("duplicate Add: got %d nodes, want 1", g.NodeCount())
	}
}

func TestGraph_BatchNewArticlesCrossEdges(t *testing.T) {
	// This test exercises the two-pass logic in Add().
	// If both A and B are new in the same batch, the graph must still
	// create edges between them. A single-pass algorithm would miss this.
	g := NewGraph()
	v := []float64{1, 0, 0}
	g.Add([]article.Article{
		makeArticle("a", "general", v, nil),
		makeArticle("b", "science", v, nil),
	})

	neighborsOfA := g.Neighbors("a", 10, 0)
	neighborsOfB := g.Neighbors("b", 10, 0)

	if len(neighborsOfA) == 0 {
		t.Error("batch new articles: a should have edges to b (two-pass logic)")
	}
	if len(neighborsOfB) == 0 {
		t.Error("batch new articles: b should have edges to a (bidirectional)")
	}
}

func TestGraph_CrossTopicNeighbors(t *testing.T) {
	// Olds' core value prop: surface connections ACROSS topics.
	// Verify the graph connects articles regardless of category.
	g := NewGraph()
	v := []float64{1, 0, 0}
	g.Add([]article.Article{
		makeArticle("tech-1", "technology", v, nil),
		makeArticle("crime-1", "crime", v, nil),
		makeArticle("sports-1", "sports", v, nil),
	})

	neighbors := g.Neighbors("tech-1", 10, 0)
	categories := make(map[string]bool)
	for _, e := range neighbors {
		// Look up the category from the graph's internal nodes.
		// We access g.nodes directly because we are in the same package.
		if art, ok := g.nodes[e.ArticleID]; ok {
			categories[art.Category] = true
		}
	}

	if !categories["crime"] {
		t.Error("cross-topic: tech-1 should connect to crime-1")
	}
	if !categories["sports"] {
		t.Error("cross-topic: tech-1 should connect to sports-1")
	}
}

// ── Race detector ──────────────────────────────────────────────────────────────

func TestGraph_ConcurrentAddAndNeighbors(t *testing.T) {
	// This test makes no assertions — its value is in running with -race.
	// go test -race ./internal/graph/... will fail if there is a data race.
	g := NewGraph()
	seed := makeArticle("seed", "general", []float64{1, 0, 0}, nil)
	g.Add([]article.Article{seed})

	var wg sync.WaitGroup

	// 10 concurrent Neighbors reads.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g.Neighbors("seed", 5, 0)
		}()
	}

	// 5 concurrent Add writes.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			art := makeArticle(
				fmt.Sprintf("concurrent-%d", n),
				"science",
				[]float64{1, 0, 0},
				nil,
			)
			g.Add([]article.Article{art})
		}(i)
	}

	wg.Wait()
}
