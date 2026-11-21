package pubsub

import (
	"sync"
	"sync/atomic"
)

// subscriberQueueSize bounds backlog for each consumer. A stalled consumer
// must not hold up the receive path or accumulate unbounded goroutines.
const subscriberQueueSize = 128

type PubSub struct {
	mu          sync.RWMutex
	subscribers []chan string
	dropped     atomic.Uint64
	closed      bool
}

func New() *PubSub {
	return &PubSub{}
}

// Subscribe returns an ordered, bounded stream of packets. A subscription made
// after Close receives an already-closed channel.
func (p *PubSub) Subscribe() <-chan string {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch := make(chan string, subscriberQueueSize)
	if p.closed {
		close(ch)
		return ch
	}
	p.subscribers = append(p.subscribers, ch)
	return ch
}

// Publish never waits for a slow subscriber. New packets are dropped for that
// subscriber when its queue is full; other subscribers still receive them.
// The caller can monitor Drops to detect overload.
func (p *PubSub) Publish(msg string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return
	}
	for _, ch := range p.subscribers {
		select {
		case ch <- msg:
		default:
			p.dropped.Add(1)
		}
	}
}

// Drops counts individual failed deliveries to subscribers due to full queues.
func (p *PubSub) Drops() uint64 {
	return p.dropped.Load()
}

// Close closes each subscriber exactly once, while excluding concurrent sends.
func (p *PubSub) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	for _, ch := range p.subscribers {
		close(ch)
	}
	p.subscribers = nil
}
