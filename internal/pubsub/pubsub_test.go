package pubsub

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPublishPreservesOrderAndIsolatesSlowSubscriber(t *testing.T) {
	p := New()
	slow := p.Subscribe()
	fast := p.Subscribe()

	// A stalled subscriber fills its bounded queue; publication remains fast.
	for i := 0; i < subscriberQueueSize; i++ {
		p.Publish("first")
		<-fast
	}
	done := make(chan struct{})
	go func() {
		p.Publish("last")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked behind a slow subscriber")
	}

	if got := p.Drops(); got != 1 {
		t.Fatalf("Drops() = %d, want 1", got)
	}
	if got := <-fast; got != "last" {
		t.Fatalf("fast subscriber got %q, want last", got)
	}
	for i := 0; i < subscriberQueueSize; i++ {
		if got := <-slow; got != "first" {
			t.Fatalf("slow subscriber got %q at index %d", got, i)
		}
	}
}

func TestCloseConcurrentWithPublish(t *testing.T) {
	p := New()
	ch := p.Subscribe()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			p.Publish("packet")
		}
	}()
	p.Close()
	p.Close()
	wg.Wait()
	for range ch {
		// Drain anything published before the close.
	}
	closed := p.Subscribe()
	if _, ok := <-closed; ok {
		t.Fatal("Subscribe after Close must return a closed channel")
	}
	p.Publish("ignored")
}

func TestConcurrentPublishersKeepSubscriberOrder(t *testing.T) {
	p := New()
	a, b := p.Subscribe(), p.Subscribe()
	const producers, messagesPerProducer = 8, 12
	var wg sync.WaitGroup
	for producer := 0; producer < producers; producer++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for n := 0; n < messagesPerProducer; n++ {
				p.Publish(fmt.Sprintf("%d:%d", id, n))
			}
		}(producer)
	}
	wg.Wait()

	// The queue is larger than the total publish count, so no drops
	// are expected and each subscriber must see the exact same order.
	if got := p.Drops(); got != 0 {
		t.Fatalf("unexpected drops: %d", got)
	}
	for n := 0; n < producers*messagesPerProducer; n++ {
		x, y := <-a, <-b
		if x != y {
			t.Fatalf("subscriber order diverged at %d: %q vs %q", n, x, y)
		}
	}
}
