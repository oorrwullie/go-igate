package main

import (
	"testing"
	"time"

	"github.com/oorrwullie/sparky/internal/log"
)

func TestSparkyRunWithoutWorkersWaitsForStop(t *testing.T) {
	logger, err := log.New()
	if err != nil {
		t.Fatal(err)
	}
	s := &Sparky{
		stop:             make(chan struct{}),
		shutdownComplete: make(chan struct{}),
		logger:           logger,
	}
	done := make(chan error, 1)
	go func() { done <- s.Run() }()

	select {
	case err := <-done:
		t.Fatalf("Run returned before shutdown: %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	s.Stop()
	s.Stop() // Repeated stop requests must never block or panic.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned an error after Stop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not finish after Stop")
	}
}
