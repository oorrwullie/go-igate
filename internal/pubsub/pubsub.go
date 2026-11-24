package pubsub

import (
	"sync"
	"sync/atomic"
)

// subscriberQueueSize bounds backlog for each consumer. A stalled consumer
// must not hold up the receive path or accumulate unbounded goroutines.
const subscriberQueueSize = 128

type PubSub struct {
	mu          sync.Mutex
	subscribers []chan string
	dropped     atomic.Uint64
	published   atomic.Uint64
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

// Publish serializes fan-out across concurrent producers, so all subscribers
// observe the same order. The short lock contains only nonblocking sends.
// Publish never waits for a slow subscriber. New packets are dropped for that
// subscriber when its queue is full; other subscribers still receive them.
// The caller can monitor Drops to detect overload.
func (p *PubSub) Publish(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	for _, ch := range p.subscribers {
		select {
		case ch <- msg:
			p.published.Add(1)
		default:
			p.dropped.Add(1)
		}
	}
}

// Published counts admitted subscriber deliveries, not unique RF packets.
// This metric uses a single atomic increment without any observer I/O.
func (p *PubSub) Published() uint64 { return p.published.Load() }

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
