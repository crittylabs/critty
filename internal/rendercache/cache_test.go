package rendercache

import "testing"

func TestCacheEvictsOldestAndKeepsRecent(t *testing.T) {
	var cache Cache[int, string]
	cache.Put(0, "first", maxBytes/2)
	cache.Put(1, "second", maxBytes/2)
	cache.Get(0)
	cache.Put(2, "third", maxBytes/2)
	if _, ok := cache.Get(1); ok {
		t.Fatal("oldest preview survived byte pressure")
	}
	for _, key := range []int{0, 2} {
		if _, ok := cache.Get(key); !ok {
			t.Fatalf("recent preview %d was discarded", key)
		}
	}
	cache.Put(0, "replacement", 1)
	cache.Put(3, "oversized", maxBytes+1)
	if value, ok := cache.Get(0); !ok || value != "replacement" {
		t.Fatal("replacement was lost")
	}
	if _, ok := cache.Get(3); ok {
		t.Fatal("oversized preview was cached")
	}
}

func TestCacheBoundsSmallResults(t *testing.T) {
	var cache Cache[int, string]
	for i := 0; i <= maxEntries; i++ {
		cache.Put(i, "error", 0)
	}
	if len(cache.items) != maxEntries {
		t.Fatal("small results exceeded entry limit")
	}
	if _, ok := cache.Get(0); ok {
		t.Fatal("oldest small result survived eviction")
	}
	if _, ok := cache.Get(maxEntries); !ok {
		t.Fatal("newest small result was discarded")
	}
}
