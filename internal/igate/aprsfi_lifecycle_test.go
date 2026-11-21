package igate

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oorrwullie/sparky/internal/log"
)

func TestAprsFiVerificationBoundedAndJoinedOnStop(t *testing.T) {
	logger, err := log.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	entered := make(chan struct{}, maxConcurrentAprsFiVerifications)
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	ig := &IGate{
		stop: make(chan struct{}),
		logger: logger,
		callSign: "N0CALL",
		httpClient: client,
		aprsFiKey: "test-key",
		aprsFiBaseURL: "https://example.invalid",
		aprsFiContext: ctx,
		aprsFiCancel: cancel,
		verifyAprsFi: true,
		disableISBeacon: true,
		aprsFiDelay: time.Millisecond,
	}
	for n := 0; n < maxConcurrentAprsFiVerifications+5; n++ {
		ig.scheduleAprsFiVerification("N0CALL>APRS:test", "test", time.Now(), false)
	}
	for n := 0; n < maxConcurrentAprsFiVerifications; n++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("verification did not enter HTTP transport")
		}
	}
	if got := len(ig.verificationSlots); got != maxConcurrentAprsFiVerifications {
		t.Fatalf("running verifications = %d, want %d", got, maxConcurrentAprsFiVerifications)
	}
	done := make(chan struct{})
	go func() { ig.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel and join in-flight verification requests")
	}
	ig.Stop()
	if got := calls.Load(); got != maxConcurrentAprsFiVerifications {
		t.Fatalf("HTTP calls = %d, want %d", got, maxConcurrentAprsFiVerifications)
	}
	if got := len(ig.verificationSlots); got != 0 {
		t.Fatalf("%d verification slots leaked", got)
	}
	ig.scheduleAprsFiVerification("N0CALL>APRS:test", "test", time.Now(), false)
	if got := calls.Load(); got != maxConcurrentAprsFiVerifications {
		t.Fatalf("new HTTP request admitted after Stop: %d", got)
	}
}

func TestAprsFiQueryCancelledByStop(t *testing.T) {
	logger, err := log.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	ig := &IGate{
		stop: make(chan struct{}),
		logger: logger,
		callSign: "N0CALL",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			close(entered)
			<-req.Context().Done()
			return nil, req.Context().Err()
		})},
		aprsFiKey: "test",
		aprsFiBaseURL: "https://example.invalid",
		aprsFiContext: ctx,
		aprsFiCancel: cancel,
	}
	errCh := make(chan error, 1)
	go func() {
		_, err := ig.latestAprsFiTimestamp()
		errCh <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("HTTP request did not start")
	}
	ig.Stop()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("request error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP request did not abort on shutdown")
	}
}
