// Package article defines the Article domain type and the in-memory Store.
//
// "Domain type" means this is the shape of an article as the rest of the
// application understands it — independent of how it came from NewsAPI or
// how it will be served as JSON. The newsapi package is responsible for
// converting NewsAPI's wire format into this type.
package article

import (
	"sync"
	"time"
)

// Entity is a named entity extracted from an article by the ML service.
// It mirrors the Entity schema in ml-service/app/schemas.py exactly.
//
// Relevant spaCy labels for news:
//   PERSON — people          ORG  — organizations
//   GPE    — cities/countries LOC  — other locations
//   EVENT  — named events    NORP — nationalities/groups
type Entity struct {
	Text  string `json:"text"`
	Label string `json:"label"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// Article represents a single news article in the Olds system.
//
// Struct tags (the backtick strings after each field) tell encoding/json what
// key name to use when marshaling to/from JSON. Without a tag, Go uses the
// field name as-is, so "PublishedAt" would become "PublishedAt" in JSON.
// With `json:"published_at"`, it becomes "published_at" — matching common
// API conventions. This is analogous to Pydantic's `alias=` or `Field(alias=...)`.
type Article struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	Source      string    `json:"source"`
	Category    string    `json:"category"`
	PublishedAt time.Time `json:"published_at"`
	// RawText stores the article body text for later use by the ML service
	// (Phase 3/4). omitempty means this field is omitted from JSON when empty —
	// the frontend doesn't need it, and it keeps responses small.
	// ImageURL is the article's lead image, provided by NewsAPI (urlToImage)
	// or The Guardian (fields.thumbnail). Optional — not all articles have one.
	ImageURL string `json:"image_url,omitempty"`

	RawText string `json:"raw_text,omitempty"`

	// Entities and Embedding are populated by the ML service (Phase 4).
	// omitempty hides them from the /articles JSON response until they are set,
	// keeping the feed payload clean while the ML service is still warming up.
	// The graph (Phase 5) reads these fields directly from the in-memory store.
	Entities  []Entity  `json:"entities,omitempty"`
	Embedding []float64 `json:"embedding,omitempty"`
}

// Store is a goroutine-safe in-memory collection of articles.
//
// Go does not have a GIL. Goroutines run on real OS threads, so when multiple
// goroutines read/write the same map concurrently, you get undefined behavior
// (or a race-detected crash). sync.RWMutex prevents this:
//
//   - Lock()/Unlock()   — exclusive write lock: only one writer at a time
//   - RLock()/RUnlock() — shared read lock: many concurrent readers allowed,
//     but not while a writer holds the write lock
//
// This is the right choice for this workload: many concurrent HTTP requests
// read articles, but only the ingestion goroutine writes.
type Store struct {
	mu       sync.RWMutex
	articles map[string]Article
}

// NewStore creates an empty Store ready for use.
//
// The Go convention for constructors is a function named New<TypeName> that
// returns a pointer to the initialized type. Returning *Store (pointer) rather
// than Store (value) means callers all share the same store in memory —
// passing a Store value would copy it, and copies of a mutex are unsafe.
func NewStore() *Store {
	return &Store{
		articles: make(map[string]Article),
	}
}

// Add upserts a slice of articles into the store, keyed by ID.
// If an article with the same ID already exists, it is overwritten.
// This gives us free deduplication — re-ingesting the same article is a no-op.
//
// The `(s *Store)` part is the "pointer receiver" — analogous to `self` in a
// Python method. The * means we operate on the actual Store, not a copy of it.
// You must use a pointer receiver whenever the method modifies the struct or
// the struct contains a mutex (copying a mutex is a bug).
func (s *Store) Add(articles []Article) {
	s.mu.Lock()
	// defer schedules s.mu.Unlock() to run when this function returns,
	// regardless of how it returns (normal return, early return, panic).
	// Think of it as a `finally` block attached to a specific call.
	// This pattern — defer unlock immediately after lock — is idiomatic Go
	// and prevents the classic bug of forgetting to unlock on an early return.
	defer s.mu.Unlock()

	for _, a := range articles {
		s.articles[a.ID] = a
	}
}

// GetAll returns a snapshot of all articles as a slice.
// The slice is a copy — callers can iterate over it safely after the lock
// is released, without holding the lock for the entire operation.
func (s *Store) GetAll() []Article {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Pre-allocate the result slice with the exact capacity we need.
	// make([]Article, 0, len(s.articles)) creates a slice with length 0
	// and capacity len(s.articles), so append never needs to reallocate.
	result := make([]Article, 0, len(s.articles))
	for _, a := range s.articles {
		result = append(result, a)
	}
	return result
}

// GetByCategory returns articles matching the given category.
func (s *Store) GetByCategory(category string) []Article {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Article
	for _, a := range s.articles {
		if a.Category == category {
			result = append(result, a)
		}
	}
	// If no articles matched, result is nil — but we return it as-is.
	// The handler layer wraps it in gin.H{"articles": result}, and Gin
	// will marshal a nil slice as null. We handle this at the handler level
	// by using a typed empty slice when nil.
	return result
}

// GetByID returns the article with the given ID and a boolean indicating
// whether it was found. The second return value (ok idiom) is Go's standard
// way to distinguish "not found" from an error — analogous to Python's
// dict.get(key, None) but with an explicit presence signal instead of None.
func (s *Store) GetByID(id string) (Article, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.articles[id]
	return a, ok
}

// Count returns the number of articles currently in the store.
// Useful for health checks and logging.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.articles)
}
