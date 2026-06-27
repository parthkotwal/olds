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
//
//	Add()      — write lock (infrequent, called at ingestion time)
//	Neighbors() — read lock  (hot path, one call per article open)
//
// Multiple concurrent Neighbors() calls proceed in parallel without blocking
// each other. Add() blocks all readers for the duration of the batch.
package graph

import (
	"log"
	"sort"
	"strings"
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

	// Connection type distribution — updated in Add() under write lock.
	// Each undirected pair is counted once.
	connEntityOnly int // jaccard > 0 but cosine == 0
	connCosineOnly int // cosine > 0 but jaccard == 0
	connBoth       int // both cosine > 0 and jaccard > 0
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

			// Compute components separately so we can track connection type.
			cosine := cosineSimilarity(newArt.Embedding, existingArt.Embedding)
			jaccard := entityJaccard(newArt.Entities, existingArt.Entities)
			w := 0.6*cosine + 0.4*jaccard
			if w <= 0 {
				continue // discard zero-weight edges — no meaningful connection
			}

			// Update connection type distribution (each undirected pair once).
			switch {
			case cosine > 0 && jaccard > 0:
				g.connBoth++
			case jaccard > 0:
				g.connEntityOnly++
			default:
				g.connCosineOnly++
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

// Related computes top-N neighbours for source by scanning a candidate slice.
// It is used as a cold-start fallback before the precomputed graph has finished
// hydrating. This is O(N) for one article, which is acceptable for a single
// request and avoids showing an empty or failed connection sidebar.
func Related(source article.Article, candidates []article.Article, topN int, minWeight float64) []Edge {
	edges := make([]Edge, 0, topN)
	for _, candidate := range candidates {
		if candidate.ID == source.ID {
			continue
		}

		w := edgeWeight(source, candidate)
		if w < minWeight {
			continue
		}

		edges = append(edges, Edge{ArticleID: candidate.ID, Weight: w})
	}

	sort.Slice(edges, func(i, j int) bool {
		return edges[i].Weight > edges[j].Weight
	})

	if topN > 0 && len(edges) > topN {
		edges = edges[:topN]
	}

	return edges
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

// GraphStats is a snapshot of the graph's topology at a point in time.
// Returned by Stats() and exposed via GET /stats for stress-test observability.
type GraphStats struct {
	NodeCount          int     `json:"node_count"`
	DirectedEdges      int     `json:"directed_edges"` // each undirected edge stored twice
	UniqueEdges        int     `json:"unique_edges"`   // undirected edge count = directed/2
	AvgEdgesPerNode    float64 `json:"avg_edges_per_node"`
	IsolatedNodes      int     `json:"isolated_nodes"` // nodes with zero edges (no connections found)
	MaxEdgesPerNode    int     `json:"max_edges_per_node"`
	DensityPct         float64 `json:"density_pct"`           // unique_edges / max_possible_edges × 100
	CrossTopicRatioPct float64 `json:"cross_topic_ratio_pct"` // % of edges that bridge different categories
}

// Stats returns a topology snapshot of the graph.
// Uses a read lock — safe to call concurrently with Neighbors() and Add().
//
// DensityPct tells us how "full" the graph is. At 100 articles with average
// density ~5%, we expect most articles to have a handful of connections. If
// density grows much faster than linearly with node count, the edge weight
// threshold (0.1) may be too permissive — a key stress-test signal.
func (g *Graph) Stats() GraphStats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodeCount := len(g.nodes)
	directedEdges := totalEdges(g.edges)
	uniqueEdges := directedEdges / 2

	var maxEdges, isolatedNodes, crossTopicDirected int
	// Single pass over nodes: compute topology stats AND cross-topic ratio.
	// Iterating over g.nodes (not g.edges) ensures isolated nodes are counted —
	// they have no entry in g.edges so a range over edges would miss them.
	for id := range g.nodes {
		edgeList := g.edges[id] // nil slice if no entry — len(nil) == 0 in Go
		count := len(edgeList)
		if count == 0 {
			isolatedNodes++
		}
		if count > maxEdges {
			maxEdges = count
		}
		// Count directed cross-topic edges originating from this node.
		// Since edges are stored bidirectionally, each undirected cross-topic
		// pair contributes 2 to crossTopicDirected — we divide by 2 below.
		sourceCategory := g.nodes[id].Category
		for _, e := range edgeList {
			if g.nodes[e.ArticleID].Category != sourceCategory {
				crossTopicDirected++
			}
		}
	}

	var avgEdges float64
	if nodeCount > 0 {
		avgEdges = float64(directedEdges) / float64(nodeCount)
	}

	// Max possible unique edges in an undirected graph = N*(N-1)/2.
	var densityPct float64
	maxPossible := nodeCount * (nodeCount - 1) / 2
	if maxPossible > 0 {
		densityPct = float64(uniqueEdges) / float64(maxPossible) * 100
	}

	// crossTopicDirected double-counts each undirected pair (A→B and B→A),
	// so unique cross-topic edges = crossTopicDirected / 2.
	// Express as a percentage of all unique edges — this is the product metric:
	// if 40% of connections bridge different categories, the engine is working.
	var crossTopicRatioPct float64
	if uniqueEdges > 0 {
		crossTopicRatioPct = float64(crossTopicDirected/2) / float64(uniqueEdges) * 100
	}

	return GraphStats{
		NodeCount:          nodeCount,
		DirectedEdges:      directedEdges,
		UniqueEdges:        uniqueEdges,
		AvgEdgesPerNode:    avgEdges,
		IsolatedNodes:      isolatedNodes,
		MaxEdgesPerNode:    maxEdges,
		DensityPct:         densityPct,
		CrossTopicRatioPct: crossTopicRatioPct,
	}
}

// ── Connection quality stats ───────────────────────────────────────────────

// ConnTypeStats holds the distribution of edge types across the graph.
// Each undirected pair is counted once.
type ConnTypeStats struct {
	EntityOnly int `json:"entity_only"` // driven by entity overlap only (cosine ≈ 0)
	CosineOnly int `json:"cosine_only"` // driven by semantic similarity only (jaccard == 0)
	Both       int `json:"both"`        // both signals contributed
}

// ConnTypes returns the current connection type distribution.
// Uses a read lock — safe to call concurrently with Add() and Neighbors().
func (g *Graph) ConnTypes() ConnTypeStats {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return ConnTypeStats{
		EntityOnly: g.connEntityOnly,
		CosineOnly: g.connCosineOnly,
		Both:       g.connBoth,
	}
}

// EntityCount tracks how many connections mention a specific entity text.
type EntityCount struct {
	Entity string `json:"entity"`
	Count  int    `json:"count"`
}

// LabelQuality tracks connection quality stats per entity label (PERSON, ORG, GPE, etc.).
// Low quality is defined as edge weight < lowQualityThreshold.
type LabelQuality struct {
	Label           string  `json:"label"`
	TotalEdges      int     `json:"total_edges"`
	LowQualityEdges int     `json:"low_quality_edges"`
	LowQualityPct   float64 `json:"low_quality_pct"`
}

// lowQualityThreshold is the edge weight below which a connection is considered
// low quality — weak semantic similarity and minimal entity overlap.
const lowQualityThreshold = 0.25

// TopEntities returns the n entity texts that appear as shared entities in the
// most connections. Called on-demand for /stats — not a hot path.
//
// For each undirected edge (A, B), shared high-signal entities are found and
// each entity's counter is incremented. Results are sorted by count descending.
func (g *Graph) TopEntities(n int) []EntityCount {
	g.mu.RLock()
	defer g.mu.RUnlock()

	counts := make(map[string]int)
	for aID, edgeList := range g.edges {
		aArt := g.nodes[aID]
		for _, e := range edgeList {
			if e.ArticleID <= aID {
				continue // process each undirected pair once (higher ID side)
			}
			bArt := g.nodes[e.ArticleID]
			setA := highSignalEntitySet(aArt.Entities)
			setB := highSignalEntitySet(bArt.Entities)
			for text := range setA {
				if _, ok := setB[text]; ok {
					counts[text]++
				}
			}
		}
	}

	result := make([]EntityCount, 0, len(counts))
	for text, cnt := range counts {
		result = append(result, EntityCount{Entity: text, Count: cnt})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Count > result[j].Count })
	if n > 0 && len(result) > n {
		result = result[:n]
	}
	return result
}

// NoisyLabels returns entity label quality stats sorted by low_quality_pct
// descending. Labels with the highest fraction of weak connections are
// candidates for filtering — a key signal identified during stress-testing
// (Phase 14: GPE/LOC entities creating noisy geographic connections).
func (g *Graph) NoisyLabels() []LabelQuality {
	g.mu.RLock()
	defer g.mu.RUnlock()

	type stat struct {
		total      int
		lowQuality int
	}
	stats := make(map[string]*stat)

	for aID, edgeList := range g.edges {
		aArt := g.nodes[aID]
		for _, e := range edgeList {
			if e.ArticleID <= aID {
				continue // each undirected pair once
			}
			bArt := g.nodes[e.ArticleID]
			isLow := e.Weight < lowQualityThreshold

			// Build normalised entity→label map for article A.
			aEntMap := make(map[string]string) // lowered text → label
			for _, ent := range aArt.Entities {
				if _, ok := highSignalLabels[ent.Label]; ok {
					aEntMap[strings.ToLower(ent.Text)] = ent.Label
				}
			}
			// For each high-signal entity in B, if it appears in A, attribute
			// the connection to that entity's label.
			for _, ent := range bArt.Entities {
				if _, ok := highSignalLabels[ent.Label]; ok {
					normText := strings.ToLower(ent.Text)
					if label, shared := aEntMap[normText]; shared {
						if stats[label] == nil {
							stats[label] = &stat{}
						}
						stats[label].total++
						if isLow {
							stats[label].lowQuality++
						}
					}
				}
			}
		}
	}

	result := make([]LabelQuality, 0, len(stats))
	for label, s := range stats {
		pct := 0.0
		if s.total > 0 {
			pct = float64(s.lowQuality) / float64(s.total) * 100
		}
		result = append(result, LabelQuality{
			Label:           label,
			TotalEdges:      s.total,
			LowQualityEdges: s.lowQuality,
			LowQualityPct:   pct,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LowQualityPct > result[j].LowQualityPct
	})
	return result
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
