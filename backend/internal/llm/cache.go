package llm

// ExplanationCache is a goroutine-safe in-memory cache for LLM-generated
// connection explanations, keyed by a canonical (sorted) article ID pair.
//
// Why cache? The same article pair is often viewed many times — for example,
// if article A and article B are both in a user's reading session. Calling the
// LLM on every WebSocket open would be wasteful and slow. The cache makes
// repeated views instant and keeps API costs near zero at steady state.
//
// The cache is in-memory only. On restart it is empty and explanations are
// regenerated on first view. This is acceptable — explanations for the same
// pair don't change (the articles don't change), so persistence would only
// help across restarts, which is a minor benefit.

import (
	"strings"
	"sync"
)

// ExplanationCache maps article ID pairs to their LLM-generated explanation.
// The RWMutex pattern is the same as article.Store: many concurrent reads
// (WebSocket handlers) are allowed simultaneously; writes (new explanations)
// take an exclusive lock momentarily.
type ExplanationCache struct {
	mu    sync.RWMutex
	store map[string]string
}

// NewExplanationCache returns a ready-to-use empty cache.
func NewExplanationCache() *ExplanationCache {
	return &ExplanationCache{
		store: make(map[string]string),
	}
}

// Get returns the cached explanation for the given article pair, if present.
// The ok idiom mirrors map lookups throughout the codebase.
func (c *ExplanationCache) Get(idA, idB string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.store[pairKey(idA, idB)]
	return v, ok
}

// Set stores the explanation for the given article pair.
// Subsequent Get calls for either ordering (A,B) or (B,A) will return it.
func (c *ExplanationCache) Set(idA, idB, explanation string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[pairKey(idA, idB)] = explanation
}

// Size returns the number of cached explanations. Useful for /stats logging.
func (c *ExplanationCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.store)
}

// pairKey produces a canonical key for an (idA, idB) pair regardless of order.
// Sorting the two IDs means (A, B) and (B, A) map to the same key — the
// explanation is the same from either direction.
//
// strings.Compare returns -1 if a < b, so we put the lexicographically smaller
// ID first. This is Go's idiomatic way to produce a deterministic ordering
// without importing sort for two items.
func pairKey(idA, idB string) string {
	if strings.Compare(idA, idB) <= 0 {
		return idA + ":" + idB
	}
	return idB + ":" + idA
}
