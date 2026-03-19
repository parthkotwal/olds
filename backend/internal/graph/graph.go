// Package graph implements an in-memory article graph for the Olds news reader.
//
// Each article is a node. Edges connect pairs of articles whose content is
// related, weighted by a combination of:
//   - Cosine similarity of their sentence-transformer embedding vectors (0.6 weight)
//   - Jaccard similarity of their high-signal named entities (0.4 weight)
//
// The combined edge weight is in [0.0, 1.0]. Edges with weight 0 are not stored.
//
// Locking model: sync.RWMutex, same pattern as article.Store.
//   Add()      — write lock (infrequent, called at ingestion time)
//   Neighbors() — read lock  (hot path, one call per article open)
//
// Multiple concurrent Neighbors() calls proceed in parallel without blocking
// each other. Add() blocks all readers for the duration of the batch.
package graph

import (
	"log"
	"sort"
	"sync"

	"github.com/olds/backend/internal/article"
)

// Edge represents a weighted directed connection to a neighbour article.
// Weight is in [0.0, 1.0]; higher means more related.
type Edge struct {
	ArticleID string  `json:"article_id"`
	Weight    float64 `json:"weight"`
}

// Graph is a goroutine-safe in-memory adjacency-list article graph.
//
// Internal layout:
//
//	nodes — full Article structs keyed by ID. The graph stores full articles
//	        (not just IDs) so it is self-contained: edge computation and
//	        neighbour lookup need only the graph, not the article.Store.
//	edges — adjacency list: article ID → slice of outgoing Edges.
//	        Edges are stored bidirectionally: A→B and B→A are both present.
//	        This trades a little extra memory for O(1) Neighbors() lookups
//	        (one map access) rather than O(N) full-graph scans.
type Graph struct {
	mu    sync.RWMutex
	nodes map[string]article.Article
	edges map[string][]Edge
}

// NewGraph returns an empty, ready-to-use Graph.
func NewGraph() *Graph {
	return &Graph{
		nodes: make(map[string]article.Article),
		edges: make(map[string][]Edge),
	}
}

// Add inserts articles as graph nodes and computes weighted edges between them
// and all existing nodes.
//
// Algorithm (two-pass to handle batches of new articles correctly):
//
//  1. Pass 1 — insert all new articles into g.nodes, collecting their IDs.
//     Skip any article whose ID is already present (deduplication, same as Store.Add).
//
//  2. Pass 2 — for each newly inserted article, compute edge weights against
//     every OTHER node in the graph (old nodes + earlier new nodes).
//     For pairs where BOTH are new: the lexicographic comparison
//     `existingID < newID` ensures we compute each pair exactly once.
//     Both directions (A→B and B→A) are stored in a single pass.
//
// Why two passes? If a batch arrives with [A, B] where neither is in the graph
// yet, a single pass would miss the A↔B edge: when A is processed, B has not
// been inserted yet, so it is not in g.nodes.
func (g *Graph) Add(articles []article.Article) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// ── Pass 1: insert new articles, collect inserted IDs ────────────────────
	var inserted []string
	for _, a := range articles {
		if _, exists := g.nodes[a.ID]; exists {
			continue // already in graph — skip (same dedup logic as Store)
		}
		g.nodes[a.ID] = a
		inserted = append(inserted, a.ID)
	}

	if len(inserted) == 0 {
		return
	}

	// Build a set of the newly inserted IDs for O(1) membership checks below.
	// In Go, map[string]struct{} is the idiomatic set — struct{} costs 0 bytes.
	insertedSet := make(map[string]struct{}, len(inserted))
	for _, id := range inserted {
		insertedSet[id] = struct{}{}
	}

	// ── Pass 2: compute edges ─────────────────────────────────────────────────
	for _, newID := range inserted {
		newArt := g.nodes[newID]

		for existingID, existingArt := range g.nodes {
			if existingID == newID {
				continue // no self-edges
			}

			// For pairs where both are newly inserted this batch, use
			// lexicographic ordering to compute each pair exactly once.
			// When newID > existingID, existingID will have already stored
			// edges to newID when existingID was processed as newID earlier
			// in this loop. Skip to avoid duplicates.
			if _, bothNew := insertedSet[existingID]; bothNew && existingID < newID {
				continue
			}

			w := edgeWeight(newArt, existingArt)
			if w <= 0 {
				continue // discard zero-weight edges — no meaningful connection
			}

			// Store both directions so Neighbors() is O(1) per node.
			g.edges[newID] = append(g.edges[newID], Edge{ArticleID: existingID, Weight: w})
			g.edges[existingID] = append(g.edges[existingID], Edge{ArticleID: newID, Weight: w})
		}
	}

	log.Printf("graph: %d nodes, %d directed edges after Add()", len(g.nodes), totalEdges(g.edges))
}

// Neighbors returns the top-N neighbours of the article with the given ID,
// filtered to those with weight >= minWeight, sorted by weight descending.
//
// Returns an empty (non-nil) slice if the article is not in the graph or has
// no neighbours above the threshold. The empty-not-nil guarantee means callers
// (and the JSON encoder) always see [] rather than null.
//
// This is the hot-path method: called once per article open by the WebSocket
// handler in Phase 8. The RLock allows many concurrent calls to proceed in
// parallel — no reader blocks another reader.
func (g *Graph) Neighbors(id string, topN int, minWeight float64) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	edges, ok := g.edges[id]
	if !ok {
		return []Edge{} // unknown article — return empty, not nil
	}

	// Filter by minimum weight threshold.
	filtered := make([]Edge, 0, len(edges))
	for _, e := range edges {
		if e.Weight >= minWeight {
			filtered = append(filtered, e)
		}
	}

	// Sort descending by weight.
	// sort.Slice is Go's standard in-place sort with a custom comparator.
	// Equivalent to Python's sorted(filtered, key=lambda e: e.Weight, reverse=True).
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Weight > filtered[j].Weight
	})

	// Limit to topN results (0 means "no limit").
	if topN > 0 && len(filtered) > topN {
		filtered = filtered[:topN]
	}

	return filtered
}

// NodeCount returns the number of articles (nodes) in the graph.
func (g *Graph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// EdgeCount returns the total number of directed edges stored.
// Since edges are stored bidirectionally, this equals 2× the number of unique
// article pairs that are connected. Useful for health checks and log output.
func (g *Graph) EdgeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return totalEdges(g.edges)
}

// totalEdges sums all edge-list lengths. Called within methods that already
// hold a lock — NOT a public method, does no locking of its own.
func totalEdges(edges map[string][]Edge) int {
	total := 0
	for _, list := range edges {
		total += len(list)
	}
	return total
}
