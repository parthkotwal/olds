package behavior

import (
	"testing"
	"time"

	"github.com/olds/backend/internal/article"
)

func TestScoreAndSortBoostsEngagedCategoryAndEntities(t *testing.T) {
	store := NewStore()
	now := time.Now()

	articles := []article.Article{
		{
			ID:          "fresh-tech",
			Title:       "Fresh tech story",
			Category:    "technology",
			PublishedAt: now.Add(-1 * time.Hour),
			Entities:    []article.Entity{{Text: "OpenAI", Label: "ORG"}},
		},
		{
			ID:          "older-world",
			Title:       "Older world story",
			Category:    "general",
			PublishedAt: now.Add(-20 * time.Hour),
			Entities:    []article.Entity{{Text: "NATO", Label: "ORG"}},
		},
		{
			ID:          "related-world",
			Title:       "Related world story",
			Category:    "general",
			PublishedAt: now.Add(-10 * time.Hour),
			Entities:    []article.Entity{{Text: "NATO", Label: "ORG"}},
		},
	}

	baseline := store.ScoreAndSort(articles)
	if baseline[0].ID != "fresh-tech" {
		t.Fatalf("expected freshest article first without behavior, got %q", baseline[0].ID)
	}

	store.Record(Event{ArticleID: "older-world", Type: EventDwell, Value: 120})
	store.Record(Event{ArticleID: "older-world", Type: EventScrollDepth, Value: 0.9})
	store.Record(Event{ArticleID: "older-world", Type: EventReopen, Value: 1})

	ranked := store.ScoreAndSort(articles)
	if ranked[0].ID != "related-world" && ranked[0].ID != "older-world" {
		t.Fatalf("expected engaged world/category/entity article to rank first, got %q", ranked[0].ID)
	}
	if ranked[0].Category != "general" {
		t.Fatalf("expected behavior to boost general category, got %q", ranked[0].Category)
	}
}

func TestRecordTracksScrollHighWaterAndOpenCount(t *testing.T) {
	store := NewStore()

	store.Record(Event{ArticleID: "a1", Type: EventScrollDepth, Value: 0.4})
	store.Record(Event{ArticleID: "a1", Type: EventScrollDepth, Value: 0.2})
	store.Record(Event{ArticleID: "a1", Type: EventScrollDepth, Value: 0.8})
	store.Record(Event{ArticleID: "a1", Type: EventReopen, Value: 1})
	store.Record(Event{ArticleID: "a1", Type: EventReopen, Value: 1})

	signals, ok := store.Signals("a1")
	if !ok {
		t.Fatal("expected signals for article")
	}
	if signals.MaxScrollDepth != 0.8 {
		t.Fatalf("expected max scroll depth 0.8, got %.2f", signals.MaxScrollDepth)
	}
	if signals.OpenCount != 2 {
		t.Fatalf("expected open count 2, got %d", signals.OpenCount)
	}
}
