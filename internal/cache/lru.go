package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Cache struct {
	capacity int
	mu       sync.Mutex
	filePath string
	lru      *LRU
	ttl      time.Duration
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

	_ = cache.loadFromFile()
	fmt.Println("Loaded cache")

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
			return true
		}
		c.lru.removeNode(node)
		delete(c.lru.cache, key)
	}

	if c.ttl > 0 {
		c.pruneExpiredLocked()
	}

	c.lru.set(key, value)
	if err := c.saveToFile(); err != nil && c.filePath != "" {
		fmt.Println("Error saving cache:", err)
	}

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

func (c *Cache) saveToFile() error {
	if c.filePath == "" {
		return nil
	}

	file, err := os.Create(c.filePath)
	if err != nil {
		return fmt.Errorf("error creating cache file: %w", err)
	}
	defer file.Close()

	data := make(map[string]int64, len(c.lru.cache))
	for k, n := range c.lru.cache {
		data[k] = n.timestamp.UnixNano()
	}

	if err := json.NewEncoder(file).Encode(data); err != nil {
		return fmt.Errorf("error encoding cache data to JSON: %w", err)
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

	now := time.Now()
	for k, ts := range data {
		timestamp := time.Unix(0, ts)
		if c.ttl > 0 && now.Sub(timestamp) > c.ttl {
			continue
		}
		c.lru.setWithTimestamp(k, timestamp)
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
