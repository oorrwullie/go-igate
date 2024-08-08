package pubsub

import "sync"

type PubSub struct {
	subscribers []chan string
	mu          sync.Mutex
}

func New() *PubSub {
	return &PubSub{
		subscribers: make([]chan string, 0),
	}
}

func (p *PubSub) Subscribe() <-chan string {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch := make(chan string)
	p.subscribers = append(p.subscribers, ch)
	return ch
}

func (p *PubSub) Publish(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, ch := range p.subscribers {
		go func(c chan string) {
			c <- msg
		}(ch)
	}
}

func (p *PubSub) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, ch := range p.subscribers {
		close(ch)
	}
}
