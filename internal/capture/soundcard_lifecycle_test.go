package capture

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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

func TestSoundcardStopCancelsActiveAplay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX stub for the aplay executable")
	}
	// A stub executable ensures the test needs neither a sound card nor aplay.
	dir := t.TempDir()
	path := filepath.Join(dir, "aplay")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithCancel(context.Background())
	s := &SoundcardCapture{
		stop:         make(chan struct{}),
		txContext:    ctx,
		txCancel:     cancel,
		txWorkerDone: make(chan struct{}),
	}
	// Simulate the running aplay worker without generating AX.25 packets.
	// Stop must cancel the child process and wait for the worker to finish.
	go func() {
		defer close(s.txWorkerDone)
		_ = s.playWithAplay([]float32{0, 0.1, -0.1})
	}()
	// Do not depend on whether the command has started when Stop arrives:
	// cancellation must work both before and during process execution.
	stopped := make(chan struct{})
	go func() {
		s.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop failed to cancel and join aplay")
	}
	s.Stop()
	if err := s.Start(); err == nil {
		t.Fatal("Start succeeded after Stop")
	}
}
