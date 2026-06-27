package llm

import (
	"strconv"
	"sync"
	"testing"
)

func TestExplanationCacheCanonicalPairKey(t *testing.T) {
	cache := NewExplanationCache()

	cache.Set("article-b", "article-a", "shared explanation")

	got, ok := cache.Get("article-a", "article-b")
	if !ok {
		t.Fatal("expected cache hit for reversed article pair")
	}
	if got != "shared explanation" {
		t.Fatalf("unexpected explanation: got %q", got)
	}
	if cache.Size() != 1 {
		t.Fatalf("expected one canonical cache entry, got %d", cache.Size())
	}
}

func TestExplanationCacheConcurrentAccess(t *testing.T) {
	cache := NewExplanationCache()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := strconv.Itoa(i % 10)
			cache.Set("a-"+id, "b-"+id, "explanation-"+id)
			if _, ok := cache.Get("b-"+id, "a-"+id); !ok {
				t.Errorf("expected cache hit for pair %s", id)
			}
		}(i)
	}
	wg.Wait()

	if cache.Size() != 10 {
		t.Fatalf("expected 10 canonical entries, got %d", cache.Size())
	}
}
