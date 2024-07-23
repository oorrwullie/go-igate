package cache

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"sync"
)

type entry struct {
	key string
	val interface{}
}

type Cache struct {
	capacity int
	cache    map[string]interface{}
	mu       sync.Mutex
	filePath string
	lru      *LRU
	bloom    *BloomFilter
}

type LRU struct {
	cache    map[string]*node
	capacity int
	head     *node
	tail     *node
}

type node struct {
	key  string
	val  interface{}
	prev *node
	next *node
}

type BloomFilter struct {
	size     uint
	hashFunc func([]byte) uint32
	bitSet   []bool
	mu       sync.Mutex
}

func NewBloomFilter(size uint) *BloomFilter {
	return &BloomFilter{
		size:     size,
		hashFunc: fnvHash,
		bitSet:   make([]bool, size),
	}
}

func (bf *BloomFilter) Add(data []byte) {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	for _, pos := range bf.getPositions(data) {
		bf.bitSet[pos] = true
	}
}

func (bf *BloomFilter) Test(data []byte) bool {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	for _, pos := range bf.getPositions(data) {
		if !bf.bitSet[pos] {
			return false
		}
	}

	return true
}

func (bf *BloomFilter) getPositions(data []byte) []uint {
	hash1, hash2 := bf.hash(data)
	return []uint{hash1 % bf.size, hash2 % bf.size}
}

func (bf *BloomFilter) hash(data []byte) (uint, uint) {
	h := fnv.New32a()
	h.Write(data)
	hash1 := uint(h.Sum32())

	h.Reset()
	h.Write(data)
	hash2 := uint(h.Sum32())

	return hash1, hash2
}

func fnvHash(data []byte) uint32 {
	h := fnv.New32a()
	h.Write(data)
	return h.Sum32()
}

func NewCache(capacity int, bloomFilterSize uint, filePath string) *Cache {
	lru := &LRU{
		cache:    make(map[string]*node),
		capacity: capacity,
	}

	cache := &Cache{
		capacity: capacity,
		cache:    make(map[string]interface{}),
		lru:      lru,
		bloom:    NewBloomFilter(bloomFilterSize),
		filePath: filePath,
	}

	cache.loadFromFile()

	return cache
}

func (l *LRU) get(key string) (interface{}, bool) {
	if node, exists := l.cache[key]; exists {
		l.moveToFront(node)
		return node.val, exists
	}

	return nil, false
}

func (l *LRU) set(key string, val interface{}) {
	if node, exists := l.cache[key]; exists {
		node.val = val
		l.moveToFront(node)
		return
	}

	if len(l.cache) >= l.capacity {
		l.evictTail()
	}

	newNode := &node{
		key: key,
		val: val,
	}

	l.cache[key] = newNode
	l.addToFront(newNode)
}

func (l *LRU) evictTail() {
	if l.tail != nil {
		delete(l.cache, l.tail.key)
		if l.tail.prev != nil {
			l.tail.prev.next = nil
		}
		l.tail = l.tail.prev
	}
}

func (l *LRU) moveToFront(n *node) {
	if n == l.head {
		return
	}

	if n.prev != nil {
		n.prev.next = n.next
	}

	if n.next != nil {
		n.next.prev = n.prev
	}

	if n == l.tail {
		l.tail = n.prev
	}

	n.next = l.head
	n.prev = nil

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

func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if value, exists := c.lru.get(key); exists {
		return value, true
	}

	return nil, false
}

func (c *Cache) Set(key string, value interface{}) (duplicate bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.bloom.Test([]byte(key)) {
		return true
	}

	c.lru.set(key, value)
	c.bloom.Add([]byte(key))
	c.saveToFile()

	return false
}

func (c *Cache) saveToFile() error {
	if c.filePath == "" {
		return fmt.Errorf("filePath cannot be empty")
	}

	file, err := os.Create(c.filePath)
	if err != nil {
		return fmt.Errorf("Error creating cache file: %v", err)
	}
	defer file.Close()

	data := make(map[string]interface{})

	for k, v := range c.lru.cache {
		data[k] = v.val
	}

	j := json.NewEncoder(file)
	err = j.Encode(data)

	if err != nil {
		return fmt.Errorf("Error encoding cache data to JSON: %v", err)
	}

	return nil
}

func (c *Cache) loadFromFile() error {
	if c.filePath == "" {
		return fmt.Errorf("filePath cannot be empty")
	}

	file, err := os.Open(c.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Error opening cache file: %v", err)
		}
	}
	defer file.Close()

	data := make(map[string]interface{})

	err = json.NewDecoder(file).Decode(&data)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Error decoding cache data from JSON: %v", err)
		}
	}

	fmt.Println("loadFromFile pre-lock")
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Println("loadFromFile post-lock")

	for k, v := range data {
		c.Set(k, v)
	}

	return nil
}
