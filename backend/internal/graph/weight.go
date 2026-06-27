package graph

// weight.go contains the functions that compute edge weights between articles,
// plus SharedEntities which is exported for use by the LLM explanation layer.
//
// All functions are pure (no side effects, no shared state) and therefore
// safe to call from concurrent goroutines without any locking.

import (
	"math"
	"sort"
	"strings"

	"github.com/olds/backend/internal/article"
)

// highSignalLabels is the set of spaCy entity labels the graph considers
// meaningful for topic connection. Stored as a map for O(1) lookup.
//
// Excluded labels: DATE, TIME, CARDINAL, ORDINAL, MONEY, QUANTITY, PERCENT —
// two articles sharing "2024" or "$5 billion" are not meaningfully related.
var highSignalLabels = map[string]struct{}{
	"PERSON": {},
	"ORG":    {},
	"GPE":    {},
	"LOC":    {},
	"EVENT":  {},
	"NORP":   {},
	"FAC":    {},
	"LAW":    {},
}

// noisyEntities is a set of entity texts that are too high-frequency to be
// useful as connection signals. These are legitimate named entities — they are
// stored in the database — but they appear in such a large fraction of articles
// that a shared match carries almost no information. Including them inflates
// Jaccard scores between unrelated articles and accounts for the majority of
// low-quality edges in the graph.
//
// Identified from /stats connection quality data: "us" and "uk" appeared in
// 6,541 and 3,853 articles respectively, driving 62% of low-quality GPE edges.
// All values are lowercase (matching the normalisation applied in highSignalEntitySet).
var noisyEntities = map[string]struct{}{
	"us":  {},
	"uk":  {},
	"u.s": {},
	// "iran" and "trump" are high-frequency but still specific enough to signal
	// a real connection — excluded from the noise list intentionally.
}

// edgeWeight combines cosine similarity and entity Jaccard overlap into a single
// weight in [0.0, 1.0]. Semantic similarity is weighted slightly higher than
// entity overlap: 0.6 × cosine + 0.4 × jaccard.
//
// The 60/40 split reflects that embedding vectors capture broader semantic
// meaning (tone, domain, narrative) whereas entity overlap is a precise but
// narrower signal. Both contribute; neither alone is sufficient.
func edgeWeight(a, b article.Article) float64 {
	cosine := cosineSimilarity(a.Embedding, b.Embedding)
	jaccard := entityJaccard(a.Entities, b.Entities)
	return 0.6*cosine + 0.4*jaccard
}

// Breakdown explains the two components that produced an edge weight.
// It is returned to the frontend for the "why connected" UI.
type Breakdown struct {
	Weight             float64  `json:"weight"`
	SemanticSimilarity float64  `json:"semantic_similarity"`
	EntityOverlap      float64  `json:"entity_overlap"`
	SemanticPct        float64  `json:"semantic_pct"`
	EntityPct          float64  `json:"entity_pct"`
	SharedEntities     []string `json:"shared_entities"`
}

// Explain returns the raw semantic/entity scores and each component's
// contribution to the final weighted edge score.
func Explain(a, b article.Article) Breakdown {
	cosine := cosineSimilarity(a.Embedding, b.Embedding)
	jaccard := entityJaccard(a.Entities, b.Entities)
	semanticContribution := 0.6 * cosine
	entityContribution := 0.4 * jaccard
	weight := semanticContribution + entityContribution

	var semanticPct, entityPct float64
	if weight > 0 {
		semanticPct = semanticContribution / weight * 100
		entityPct = entityContribution / weight * 100
	}

	return Breakdown{
		Weight:             weight,
		SemanticSimilarity: cosine,
		EntityOverlap:      jaccard,
		SemanticPct:        semanticPct,
		EntityPct:          entityPct,
		SharedEntities:     SharedEntities(a, b),
	}
}

// cosineSimilarity computes the cosine similarity between two float64 vectors.
//
// Cosine similarity = dot(a,b) / (|a| × |b|)
//
// It measures the angle between two vectors in high-dimensional space,
// independent of their magnitudes. Two articles about the same topic from
// different sources will have high cosine similarity even if one article is
// much longer (longer text → larger magnitude vector without normalisation,
// but the direction — which is what cosine measures — stays similar).
//
// Returns 0.0 for any degenerate inputs:
//   - nil or empty slice
//   - mismatched lengths (defensive: the model always returns 384 dims, but
//     we must not panic if the ML service behaves unexpectedly)
//   - zero-magnitude vector (the zero vector has no direction)
func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0.0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	// math.Sqrt is Go's standard library square root — same as Python's math.sqrt.
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// entityJaccard computes the Jaccard similarity of the high-signal entities
// from two articles.
//
//	Jaccard(A, B) = |A ∩ B| / |A ∪ B|
//
// Only entities whose Label is in highSignalLabels are counted. Entity text is
// normalised to lowercase so "Apple" and "apple" are treated as the same entity.
//
// Returns 0.0 if both articles have no high-signal entities — avoids 0/0
// and correctly treats two contentless articles as unrelated.
func entityJaccard(a, b []article.Entity) float64 {
	setA := highSignalEntitySet(a)
	setB := highSignalEntitySet(b)

	if len(setA) == 0 && len(setB) == 0 {
		return 0.0
	}

	var intersection int
	for text := range setA {
		if _, ok := setB[text]; ok {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// SharedEntities returns the high-signal entity texts that appear in both
// articles. Called by the LLM layer to construct the connection explanation
// prompt — "shared entities: Xi Jinping, South China Sea" tells the model
// exactly what drove the edge, producing more precise explanations.
//
// The returned slice is sorted for deterministic output (consistent prompts
// → more consistent LLM responses, and easier caching if needed later).
func SharedEntities(a, b article.Article) []string {
	setA := highSignalEntitySet(a.Entities)
	setB := highSignalEntitySet(b.Entities)

	var shared []string
	for text := range setA {
		if _, ok := setB[text]; ok {
			shared = append(shared, text)
		}
	}
	sort.Strings(shared) // deterministic ordering
	return shared
}

// highSignalEntitySet builds a set of normalised entity text strings from a
// slice, filtering out low-signal labels. The map[string]struct{} type is Go's
// idiomatic set — the empty struct{} value costs zero bytes.
//
// Normalisation: strings.ToLower so "UN" and "un" are the same entity.
// We do not strip punctuation — "U.S." and "US" will NOT match. Acceptable
// for Phase 5; a normaliser could be added later if precision drops.
func highSignalEntitySet(entities []article.Entity) map[string]struct{} {
	set := make(map[string]struct{})
	for _, e := range entities {
		if _, ok := highSignalLabels[e.Label]; !ok {
			continue
		}
		norm := strings.ToLower(e.Text)
		if _, noisy := noisyEntities[norm]; noisy {
			continue
		}
		set[norm] = struct{}{}
	}
	return set
}
