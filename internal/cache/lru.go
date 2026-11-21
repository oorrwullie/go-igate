package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const snapshotInterval = 5 * time.Second

type Cache struct {
	capacity int
	mu       sync.Mutex
	filePath string
	lru      *LRU
	ttl      time.Duration
	lastSnapshot time.Time
	dirty bool
}

type LRU struct {
	cache    map[string]*node
	capacity int
	head     *node
	tail     *node
}

type node struct {
	key       string
	val       interface{}
	prev      *node
	next      *node
	timestamp time.Time
}

func NewCache(capacity int, filePath string, ttl time.Duration) *Cache {
	if capacity <= 0 {
		capacity = 1
	}

	lru := &LRU{
		cache:    make(map[string]*node),
		capacity: capacity,
	}

	cache := &Cache{
		capacity: capacity,
		filePath: filePath,
		lru:      lru,
		ttl:      ttl,
	}

	if err := cache.loadFromFile(); err != nil {
		fmt.Println("Error loading cache:", err)
	}

	return cache
}

func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	node, exists := c.lru.cache[key]
	if !exists {
		return nil, false
	}

	if c.ttl > 0 && time.Since(node.timestamp) > c.ttl {
		c.lru.removeNode(node)
		delete(c.lru.cache, key)
		return nil, false
	}

	c.lru.moveToFront(node)
	return node.val, true
}

func (c *Cache) Set(key string, value interface{}) (duplicate bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node, exists := c.lru.cache[key]; exists {
		if c.ttl <= 0 || time.Since(node.timestamp) <= c.ttl {
			node.timestamp = time.Now()
			node.val = value
			c.lru.moveToFront(node)
			c.markDirtyLocked()
			return true
		}
		c.lru.removeNode(node)
		delete(c.lru.cache, key)
	}

	if c.ttl > 0 {
		c.pruneExpiredLocked()
	}

	c.lru.set(key, value)
	c.markDirtyLocked()

	return false
}

func (c *Cache) pruneExpiredLocked() {
	if c.ttl <= 0 {
		return
	}

	now := time.Now()
	for key, node := range c.lru.cache {
		if now.Sub(node.timestamp) > c.ttl {
			c.lru.removeNode(node)
			delete(c.lru.cache, key)
		}
	}
}

// markDirtyLocked limits full-file writes on the RF receive hot path.
// Cache snapshots are intentionally best-effort: a crash can lose at most the
// activity since the last successful snapshot (or longer if persistence
// fails). Flush forces a final snapshot on a clean shutdown.
func (c *Cache) markDirtyLocked() {
	if c.filePath == "" {
		return
	}
	c.dirty = true
	if !c.lastSnapshot.IsZero() && time.Since(c.lastSnapshot) < snapshotInterval {
		return
	}
	c.persistLocked()
}

// Flush persists all pending mutations. Call after stopping producers on
// orderly shutdown. It is safe to call concurrently with Set and Get.
func (c *Cache) Flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.dirty {
		return nil
	}
	return c.saveDirtyLocked()
}

func (c *Cache) persistLocked() {
	if err := c.saveDirtyLocked(); err != nil {
		fmt.Println("Error saving cache:", err)
	}
}

func (c *Cache) saveDirtyLocked() error {
	// A failed write is not retried for every incoming frame.
	c.lastSnapshot = time.Now()
	if err := c.saveToFile(); err != nil {
		return err
	}
	c.dirty = false
	return nil
}

func (c *Cache) saveToFile() error {
	if c.filePath == "" {
		return nil
	}
	data := make(map[string]int64, len(c.lru.cache))
	for k, n := range c.lru.cache {
		data[k] = n.timestamp.UnixNano()
	}
	file, err := os.CreateTemp(filepath.Dir(c.filePath), ".sparky-cache-*")
	if err != nil {
		return fmt.Errorf("create cache temp file: %w", err)
	}
	defer os.Remove(file.Name())
	if err := json.NewEncoder(file).Encode(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode cache data: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync cache data: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close cache temp file: %w", err)
	}
	if err := os.Rename(file.Name(), c.filePath); err != nil {
		return fmt.Errorf("replace cache file: %w", err)
	}
	return nil
}

func (c *Cache) loadFromFile() error {
	if c.filePath == "" {
		return nil
	}

	file, err := os.Open(c.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("error opening cache file: %w", err)
		}
		return nil
	}
	defer file.Close()

	data := make(map[string]int64)
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("error decoding cache data from JSON: %w", err)
	}

	// Restore chronological admission order, not randomized map order.
	type entry struct {
		key string
		ts  time.Time
	}
	entries := make([]entry, 0, len(data))
	now := time.Now()
	for k, ts := range data {
		timestamp := time.Unix(0, ts)
		if c.ttl > 0 && now.Sub(timestamp) > c.ttl {
			continue
		}
		entries = append(entries, entry{key: k, ts: timestamp})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ts.Equal(entries[j].ts) {
			return entries[i].key < entries[j].key
		}
		return entries[i].ts.Before(entries[j].ts)
	})
	for _, e := range entries {
		c.lru.setWithTimestamp(e.key, e.ts)
	}

	return nil
}

func (l *LRU) set(key string, val interface{}) {
	if node, exists := l.cache[key]; exists {
		node.val = val
		node.timestamp = time.Now()
		l.moveToFront(node)
		return
	}

	if len(l.cache) >= l.capacity {
		l.evictTail()
	}

	newNode := &node{
		key:       key,
		val:       val,
		timestamp: time.Now(),
	}

	l.cache[key] = newNode
	l.addToFront(newNode)
}

func (l *LRU) setWithTimestamp(key string, ts time.Time) {
	if len(l.cache) >= l.capacity {
		l.evictTail()
	}

	newNode := &node{
		key:       key,
		val:       ts,
		timestamp: ts,
	}

	l.cache[key] = newNode
	l.addToFront(newNode)
}

func (l *LRU) evictTail() {
	if l.tail != nil {
		delete(l.cache, l.tail.key)
		l.removeNode(l.tail)
	}
}

func (l *LRU) removeNode(n *node) {
	if n.prev != nil {
		n.prev.next = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	}
	if l.head == n {
		l.head = n.next
	}
	if l.tail == n {
		l.tail = n.prev
	}
	n.prev = nil
	n.next = nil
}

func (l *LRU) moveToFront(n *node) {
	if n == l.head {
		return
	}

	l.removeNode(n)
	n.next = l.head
	if l.head != nil {
		l.head.prev = n
	}
	l.head = n
	if l.tail == nil {
		l.tail = n
	}
}

func (l *LRU) addToFront(n *node) {
	if l.head == nil {
		l.head = n
		l.tail = n
		return
	}

	n.next = l.head
	l.head.prev = n
	l.head = n
}
