package capture

import (
	"testing"
	"time"

	"github.com/oorrwullie/sparky/internal/log"
)

// The audio queue is intentionally bounded; stopping must release a sender
// awaiting capacity without closing a channel that other senders may use.
func TestSoundcardPlayUnblocksOnStop(t *testing.T) {
	logger, err := log.New()
	if err != nil {
		t.Fatal(err)
	}
	s := &SoundcardCapture{
		stop:        make(chan struct{}),
		sendChannel: make(chan string, 1),
		logger:      logger,
	}
	s.sendChannel <- "first"
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Play("second")
	}()
	select {
	case <-done:
		t.Fatal("Play unexpectedly returned with a full queue")
	case <-time.After(30 * time.Millisecond):
	}

	s.Stop()
	s.Stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("blocked Play was not canceled")
	}
	// Safe to reject sends after stopping; never panic on a closed queue.
	s.Play("third")
}
