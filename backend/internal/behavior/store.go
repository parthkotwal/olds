// Package behavior tracks implicit reading signals and uses them to re-rank
// the article feed. No explicit ratings — the system learns from how you read.
//
// Three signals are tracked:
//   - Dwell time: seconds spent reading an article
//   - Scroll depth: how far through the article the user scrolled (0.0–1.0)
//   - Open count: number of times the user has opened the article
//
// These signals drive two affinity scores:
//   - Category affinity: categories with more total dwell time are surfaced higher
//   - Entity affinity: entities appearing in heavily-read articles are surfaced higher
//
// The scoring formula is:
//
//	score = 1.0
//	      + 0.6 × (categoryDwell / maxCategoryDwell)
//	      + 0.4 × (meanEntityDwell / maxEntityDwell)
//
// A score of 1.0 means no behavioral signal — unread articles always appear
// rather than being buried. The boosts are additive, so the range is [1.0, 2.0].
package behavior

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/olds/backend/internal/article"
)

// EventType is the kind of behavioral signal being recorded.
// Using a string type (rather than iota int) makes JSON payloads
// self-documenting: {"type":"dwell"} is clearer than {"type":1}.
type EventType = string

const (
	EventDwell       EventType = "dwell"        // value = seconds (float64)
	EventScrollDepth EventType = "scroll_depth" // value = 0.0–1.0 fraction
	EventReopen      EventType = "reopen"       // value = 1.0 (count increment)
)

// Event is the payload sent by the frontend to POST /behavior.
// It mirrors the JSON the browser sends, validated in the handler.
//
// UserID is NOT populated from the request body — it is set by the handler
// after extracting the verified user ID from the JWT via the auth middleware.
// The json:"-" tag ensures it is never accidentally read from or written to JSON.
type Event struct {
	ArticleID string    `json:"article_id"`
	Type      EventType `json:"type"`
	Value     float64   `json:"value"`
	UserID    string    `json:"-"` // set from JWT claims, never from request body
}

// ArticleSignals holds the accumulated behavioral signals for a single article.
// Values are updated in place — never replaced — so concurrent reads see a
// monotonically increasing picture of engagement.
type ArticleSignals struct {
	TotalDwell     float64 // cumulative seconds across all sessions
	MaxScrollDepth float64 // 0.0–1.0; highest depth reached across all sessions
	OpenCount      int     // total number of times the article was opened
}

// Store is a goroutine-safe in-memory accumulator of behavioral signals.
// It is intentionally separate from article.Store — it owns engagement data,
// not article content. The separation keeps each package focused and testable.
type Store struct {
	mu      sync.RWMutex
	signals map[string]*ArticleSignals // keyed by article ID
}

// NewStore returns an empty, ready-to-use behavior Store.
func NewStore() *Store {
	return &Store{
		signals: make(map[string]*ArticleSignals),
	}
}

// Record accumulates a single behavioral event.
// Concurrent calls are safe — the write lock is held only for the map update.
func (s *Store) Record(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get-or-create the signals entry for this article.
	// This is the Go idiom for "insert if absent":
	//   sig, ok := s.signals[e.ArticleID]
	//   if !ok { sig = &ArticleSignals{}; s.signals[e.ArticleID] = sig }
	// The comma-ok form is Go's equivalent of Python's dict.get(key).
	sig, ok := s.signals[e.ArticleID]
	if !ok {
		sig = &ArticleSignals{}
		s.signals[e.ArticleID] = sig
	}

	switch e.Type {
	case EventDwell:
		sig.TotalDwell += e.Value
	case EventScrollDepth:
		// MaxScrollDepth is the high-water mark, not a sum.
		// A user who scrolled to 80% on three visits still gets 0.80, not 2.40.
		if e.Value > sig.MaxScrollDepth {
			sig.MaxScrollDepth = e.Value
		}
	case EventReopen:
		sig.OpenCount++
	}
}

// decayScore computes a time-based relevance multiplier for an article.
//
// Uses exponential decay with a 24-hour half-life:
//
//	decay = 0.5 ^ (age_in_hours / 24)
//
// This produces:
//
//	 0h → 1.00   (just published)
//	12h → 0.71
//	24h → 0.50   (half of original score)
//	48h → 0.25
//	72h → 0.13
//
// The minimum is clamped to 0.05 so even very old articles don't completely
// disappear — they can still be surfaced by strong behavioral signals.
//
// math.Exp2(x) computes 2^x. We use -age/halfLife to get 0.5^(age/halfLife):
//
//	0.5^n = 2^(-n) = math.Exp2(-n)
func decayScore(publishedAt time.Time) float64 {
	const halfLifeHours = 24.0
	const minScore = 0.05

	ageHours := time.Since(publishedAt).Hours()
	if ageHours < 0 {
		// Future-dated articles (clock skew) treated as brand new.
		ageHours = 0
	}

	score := math.Exp2(-ageHours / halfLifeHours)
	if score < minScore {
		score = minScore
	}
	return score
}

// ScoreAndSort re-ranks a slice of articles by combining time decay with
// behavioral affinity signals. Returns a new sorted slice; original is unchanged.
//
// Scoring formula:
//
//	score = decay(publishedAt)
//	      + 0.6 × (categoryDwell / maxCategoryDwell)
//	      + 0.4 × (meanEntityDwell / maxEntityDwell)
//
// decay alone means: fresh articles with no behavioral signal appear at the top
// by default; they fade over time unless the user engages with their category/entities.
//
// Behavioral signals counteract decay: an article from a category the user
// reads heavily will outscore a fresh article in an ignored category.
//
// The caller (handler.List) passes all articles for the current category
// filter and gets back the re-ranked version.
func (s *Store) ScoreAndSort(articles []article.Article) []article.Article {
	if len(articles) == 0 {
		return articles
	}

	// Take a read snapshot of signals to avoid holding the lock during scoring.
	s.mu.RLock()
	signalSnapshot := make(map[string]ArticleSignals, len(s.signals))
	for id, sig := range s.signals {
		signalSnapshot[id] = *sig // copy value, not pointer
	}
	s.mu.RUnlock()

	// ── Build category affinity map ───────────────────────────────────────────
	// For each category, sum the total dwell of all articles in that category.
	// This tells us how interested the user is in each topic area.
	categoryDwell := make(map[string]float64)
	for _, a := range articles {
		if sig, ok := signalSnapshot[a.ID]; ok {
			categoryDwell[a.Category] += sig.TotalDwell
		}
	}

	// Find the maximum so we can normalize to [0, 1].
	// Normalization ensures category and entity affinity are on the same scale.
	maxCategoryDwell := maxFloat(categoryDwell)

	// ── Build entity affinity map ─────────────────────────────────────────────
	// For each entity (by text), sum the total dwell of all articles that
	// contain that entity. "South China Sea" appearing in a heavily-read article
	// boosts other articles that also mention it.
	entityDwell := make(map[string]float64)
	for _, a := range articles {
		sig, ok := signalSnapshot[a.ID]
		if !ok || sig.TotalDwell == 0 {
			continue
		}
		for _, ent := range a.Entities {
			entityDwell[ent.Text] += sig.TotalDwell
		}
	}
	maxEntityDwell := maxFloat(entityDwell)

	// ── Score each article ────────────────────────────────────────────────────
	type scored struct {
		article article.Article
		score   float64
	}
	scoredArticles := make([]scored, len(articles))

	for i, a := range articles {
		// Time-decay base: fresh articles start at 1.0 and halve every 24h.
		// Behavioral boosts (below) are additive — they can partially or fully
		// offset decay for articles in topics the user actively engages with.
		score := decayScore(a.PublishedAt)

		// Category affinity term (weight 0.6)
		if maxCategoryDwell > 0 {
			score += 0.6 * (categoryDwell[a.Category] / maxCategoryDwell)
		}

		// Entity affinity term (weight 0.4)
		// Use the mean entity dwell across this article's entities so that
		// articles with many entities don't get an unfair advantage.
		if maxEntityDwell > 0 && len(a.Entities) > 0 {
			var entitySum float64
			for _, ent := range a.Entities {
				entitySum += entityDwell[ent.Text]
			}
			meanEntityDwell := entitySum / float64(len(a.Entities))
			score += 0.4 * (meanEntityDwell / maxEntityDwell)
		}

		scoredArticles[i] = scored{article: a, score: score}
	}

	// Stable sort: articles with equal scores preserve their original order
	// (which is ingestion order = recency). This means behavioral re-ranking
	// only changes order when there's a meaningful signal difference.
	sort.SliceStable(scoredArticles, func(i, j int) bool {
		return scoredArticles[i].score > scoredArticles[j].score
	})

	// Extract sorted articles into a fresh slice.
	result := make([]article.Article, len(scoredArticles))
	for i, s := range scoredArticles {
		result[i] = s.article
	}
	return result
}

// BulkLoad replaces the current signals map with the provided snapshot.
// Called once at startup by repository.HydrateFromDB after loading aggregated
// signals from Postgres. This restores the feed ranking state that existed
// before the server was last restarted.
//
// Not safe to call concurrently with Record() — call it only during the
// single-threaded startup phase, before the HTTP server begins serving requests.
func (s *Store) BulkLoad(signals map[string]ArticleSignals) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sig := range signals {
		// In Go, ranging over a map gives copies of the values. We copy sig
		// into a new variable and store a pointer to it. This is the Go idiom
		// for "store a pointer to a struct value" — you cannot take the address
		// of a map value directly (maps don't expose their internal storage).
		copy := sig
		s.signals[id] = &copy
	}
}

// Signals returns a snapshot of accumulated signals for a given article ID.
// Returns a zero-value ArticleSignals and false if no data has been recorded.
// Useful for the health/debug endpoint.
func (s *Store) Signals(articleID string) (ArticleSignals, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sig, ok := s.signals[articleID]
	if !ok {
		return ArticleSignals{}, false
	}
	return *sig, true
}

// maxFloat returns the maximum value in a map[string]float64.
// Returns 0 if the map is empty — callers guard against dividing by 0.
func maxFloat(m map[string]float64) float64 {
	var max float64
	for _, v := range m {
		if v > max {
			max = v
		}
	}
	return max
}
