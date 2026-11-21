package transmitter

import (
	"sync"
	"testing"
	"time"

	"github.com/oorrwullie/sparky/internal/log"
)

type fakeBackend struct {
	mu       sync.Mutex
	messages []string
	entered  chan struct{}
	unblock  chan struct{}
	once     sync.Once
}

func (b *fakeBackend) Play(msg string) {
	if b.entered != nil {
		b.once.Do(func() { close(b.entered) })
	}
	if b.unblock != nil {
		<-b.unblock
	}
	b.mu.Lock()
	b.messages = append(b.messages, msg)
	b.mu.Unlock()
}

func (b *fakeBackend) Stop() {
	if b.unblock != nil {
		select {
		case <-b.unblock:
		default:
			close(b.unblock)
		}
	}
}

func TestTransmitterWorkerPreservesOrder(t *testing.T) {
	logger, err := log.New()
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{}
	tx, err := NewWithBackend(backend, logger)
	if err != nil {
		t.Fatal(err)
	}
	if !tx.Tx.TrySend("first") {
		t.Fatal("initial frame not queued")
	}
	if err := tx.Start(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Start(); err != nil {
		t.Fatal("repeated Start should not spawn another worker:", err)
	}

	// Wait for the first frame before admitting the next; a single worker
	// must dispatch in queue order.
	deadline := time.After(time.Second)
	for {
		backend.mu.Lock()
		n := len(backend.messages)
		backend.mu.Unlock()
		if n == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("first frame was not delivered")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if !tx.Tx.TrySend("second") {
		t.Fatal("second frame not queued")
	}
	for {
		backend.mu.Lock()
		n := len(backend.messages)
		backend.mu.Unlock()
		if n == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("second frame was not delivered")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	tx.Stop()
	tx.Stop()
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.messages) != 2 || backend.messages[0] != "first" || backend.messages[1] != "second" {
		t.Fatalf("unexpected dispatch sequence: %v", backend.messages)
	}
	if tx.Tx.TrySend("after stop") {
		t.Fatal("admission succeeded after Stop")
	}
}

func TestTransmitterStopReleasesBlockedBackend(t *testing.T) {
	logger, err := log.New()
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{entered: make(chan struct{}), unblock: make(chan struct{})}
	tx, err := NewWithBackend(backend, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Start(); err != nil {
		t.Fatal(err)
	}
	if !tx.Tx.TrySend("frame") {
		t.Fatal("frame not queued")
	}
	select {
	case <-backend.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not enter backend")
	}
	done := make(chan struct{})
	go func() { tx.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not release blocked backend")
	}
	if tx.Tx.SendUntil("later", nil) {
		t.Fatal("SendUntil accepted after Stop")
	}
	if err := tx.Start(); err == nil {
		t.Fatal("Start succeeded after Stop")
	}
}

func TestTransmitterStopBeforeStart(t *testing.T) {
	logger, err := log.New()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := NewWithBackend(&fakeBackend{}, logger)
	if err != nil {
		t.Fatal(err)
	}
	tx.Stop()
	if err := tx.Start(); err == nil {
		t.Fatal("Start succeeded after Stop")
	}
}
