package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLRUEvictionAndRecency(t *testing.T) {
	c := NewCache(2, "", time.Hour)
	c.Set("a", "A")
	c.Set("b", "B")
	if value, ok := c.Get("a"); !ok || value != "A" {
		t.Fatalf("Get(a) = %v, %v", value, ok)
	}
	c.Set("c", "C")
	if _, ok := c.Get("b"); ok {
		t.Fatal("least recently used b was not evicted")
	}
	for _, key := range []string{"a", "c"} {
		if _, ok := c.Get(key); !ok {
			t.Fatalf("expected %s retained", key)
		}
	}
	if len(c.lru.cache) != 2 {
		t.Fatalf("cache has %d entries, capacity 2", len(c.lru.cache))
	}
}

func TestCacheTTLRefreshSurvivesRestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cache.json")
	c := NewCache(2, file, 200*time.Millisecond)
	if c.Set("frame", time.Now()) {
		t.Fatal("first Set should not be duplicate")
	}
	time.Sleep(110 * time.Millisecond)
	if !c.Set("frame", time.Now()) {
		t.Fatal("second Set should be duplicate")
	}
	time.Sleep(110 * time.Millisecond)
	restored := NewCache(2, file, 200*time.Millisecond)
	if _, ok := restored.Get("frame"); !ok {
		t.Fatal("refreshed duplicate expired after restart")
	}
}

func TestRestoreKeepsMostRecentEntries(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cache.json")
	now := time.Now()
	data := map[string]int64{
		"old": now.Add(-3*time.Minute).UnixNano(),
		"mid": now.Add(-2*time.Minute).UnixNano(),
		"new": now.Add(-time.Minute).UnixNano(),
	}
	bytes, err := json.Marshal(data)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(file, bytes, 0o600); err != nil { t.Fatal(err) }
	for n := 0; n < 20; n++ {
		c := NewCache(2, file, 0)
		if _, ok := c.Get("old"); ok { t.Fatal("old entry incorrectly retained") }
		for _, key := range []string{"mid", "new"} {
			if _, ok := c.Get(key); !ok { t.Fatalf("missing %s", key) }
		}
	}
}

func TestCacheConcurrentBound(t *testing.T) {
	c := NewCache(8, "", time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for n := 0; n < 200; n++ {
				key := string(rune('a' + (id+n)%24))
				c.Set(key, n)
				c.Get(key)
			}
		}(i)
	}
	wg.Wait()
	if len(c.lru.cache) > 8 { t.Fatalf("capacity exceeded: %d", len(c.lru.cache)) }
	seen := make(map[*node]bool)
	var tail *node
	for n := c.lru.head; n != nil; n = n.next {
		if seen[n] { t.Fatal("cycle in LRU list") }
		seen[n] = true
		tail = n
	}
	if len(seen) != len(c.lru.cache) || tail != c.lru.tail {
		t.Fatalf("linked list and map inconsistent: nodes=%d map=%d", len(seen), len(c.lru.cache))
	}
}
