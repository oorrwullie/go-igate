package pubsub

import (
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
