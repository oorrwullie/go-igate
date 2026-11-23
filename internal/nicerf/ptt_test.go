package nicerf

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

type fakePTT struct {
	mu      sync.Mutex
	events  []bool
	failOn  map[bool]error
	entered chan struct{}
	once    sync.Once
}

func (f *fakePTT) SetTransmit(ctx context.Context, enabled bool) error {
	f.mu.Lock()
	f.events = append(f.events, enabled)
	err := f.failOn[enabled]
	f.mu.Unlock()
	if enabled && f.entered != nil {
		f.once.Do(func() { close(f.entered) })
	}
	return err
}

func (f *fakePTT) history() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]bool(nil), f.events...)
}

type fakeAudio struct {
	mu      sync.Mutex
	frames  []string
	entered chan struct{}
	release chan struct{}
	fail    error
	once    sync.Once
}

func (a *fakeAudio) PlayFrame(ctx context.Context, frame string) error {
	a.mu.Lock()
	a.frames = append(a.frames, frame)
	a.mu.Unlock()
	if a.entered != nil {
		a.once.Do(func() { close(a.entered) })
	}
	if a.release != nil {
		select {
		case <-a.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return a.fail
}

func mustController(t *testing.T, g *fakePTT, a *fakeAudio) *PTTController {
	t.Helper()
	c, err := NewPTTController(g, a)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestPTTRequiresArmAndHasOrderedSequence(t *testing.T) {
	g, a := &fakePTT{}, &fakeAudio{}
	c := mustController(t, g, a)
	if err := c.Transmit(context.Background(), "frame"); !errors.Is(err, ErrNotArmed) {
		t.Fatalf("got %v", err)
	}
	if err := c.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Transmit(context.Background(), "frame"); err != nil {
		t.Fatal(err)
	}
	if got := g.history(); !reflect.DeepEqual(got, []bool{false, true, false}) {
		t.Fatalf("sequence %v", got)
	}
	if !reflect.DeepEqual(a.frames, []string{"frame"}) {
		t.Fatalf("audio %v", a.frames)
	}
}

func TestPTTCancellationReleases(t *testing.T) {
	g, a := &fakePTT{}, &fakeAudio{entered: make(chan struct{}), release: make(chan struct{})}
	c := mustController(t, g, a)
	if err := c.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- c.Transmit(ctx, "frame") }()
	select {
	case <-a.entered:
	case <-time.After(time.Second):
		t.Fatal("never started")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("not cancelled")
	}
	if got := g.history(); !reflect.DeepEqual(got, []bool{false, true, false}) {
		t.Fatalf("sequence %v", got)
	}
}

func TestPTTCloseInterruptsPlayback(t *testing.T) {
	g, a := &fakePTT{}, &fakeAudio{entered: make(chan struct{}), release: make(chan struct{})}
	c := mustController(t, g, a)
	if err := c.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- c.Transmit(context.Background(), "frame") }()
	select {
	case <-a.entered:
	case <-time.After(time.Second):
		t.Fatal("not entered")
	}
	done := make(chan error, 1)
	go func() { done <- c.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("close blocked")
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("transmit err %v", err)
	}
	if got := g.history(); !reflect.DeepEqual(got, []bool{false, true, false, false}) {
		t.Fatalf("sequence %v", got)
	}
	if err := c.Transmit(context.Background(), "next"); !errors.Is(err, ErrPTTClosed) {
		t.Fatalf("closed: %v", err)
	}
}

func TestPTTKeyErrorStillReleases(t *testing.T) {
	g, a := &fakePTT{failOn: map[bool]error{true: errors.New("key failed")}}, &fakeAudio{}
	c := mustController(t, g, a)
	if err := c.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Transmit(context.Background(), "frame"); err == nil {
		t.Fatal("accepted key failure")
	}
	if got := g.history(); !reflect.DeepEqual(got, []bool{false, true, false}) {
		t.Fatalf("sequence %v", got)
	}
	if len(a.frames) != 0 {
		t.Fatal("played without key")
	}
}

func TestPTTReleaseErrorFaults(t *testing.T) {
	g, a := &fakePTT{}, &fakeAudio{}
	c := mustController(t, g, a)
	if err := c.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	g.mu.Lock()
	g.failOn = map[bool]error{false: errors.New("release failed")}
	g.mu.Unlock()
	if err := c.Transmit(context.Background(), "frame"); !errors.Is(err, ErrPTTFault) {
		t.Fatalf("got %v", err)
	}
	if err := c.Transmit(context.Background(), "again"); !errors.Is(err, ErrPTTFault) {
		t.Fatalf("got %v", err)
	}
	if len(a.frames) != 1 {
		t.Fatal("allowed second frame after release fault")
	}
}

func TestPTTConcurrentAdmissionIsSerialized(t *testing.T) {
	g, a := &fakePTT{}, &fakeAudio{entered: make(chan struct{}), release: make(chan struct{})}
	c := mustController(t, g, a)
	if err := c.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	one := make(chan error, 1)
	go func() { one <- c.Transmit(context.Background(), "first") }()
	select {
	case <-a.entered:
	case <-time.After(time.Second):
		t.Fatal("not entered")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := c.Transmit(ctx, "second"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting second %v", err)
	}
	close(a.release)
	if err := <-one; err != nil {
		t.Fatal(err)
	}
	if err := c.Transmit(context.Background(), "third"); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !reflect.DeepEqual(a.frames, []string{"first", "third"}) {
		t.Fatalf("frames %v", a.frames)
	}
}

func TestPTTAudioErrorReleases(t *testing.T) {
	g, a := &fakePTT{}, &fakeAudio{fail: errors.New("DAC failure")}
	c := mustController(t, g, a)
	if err := c.Arm(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Transmit(context.Background(), "frame"); err == nil {
		t.Fatal("accepted player failure")
	}
	if got := g.history(); !reflect.DeepEqual(got, []bool{false, true, false}) {
		t.Fatalf("sequence %v", got)
	}
}

func TestPTTArmFailureNeverKeys(t *testing.T) {
	g, a := &fakePTT{failOn: map[bool]error{false: errors.New("broken off control")}}, &fakeAudio{}
	c := mustController(t, g, a)
	if err := c.Arm(context.Background()); !errors.Is(err, ErrPTTFault) {
		t.Fatalf("arm %v", err)
	}
	if err := c.Transmit(context.Background(), "frame"); !errors.Is(err, ErrPTTFault) {
		t.Fatalf("transmit %v", err)
	}
	for _, enabled := range g.history() {
		if enabled {
			t.Fatal("asserted PTT after arm failure")
		}
	}
}

func TestPTTCloseIdempotent(t *testing.T) {
	g, a := &fakePTT{}, &fakeAudio{}
	c := mustController(t, g, a)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	count := len(g.history())
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if len(g.history()) != count {
		t.Fatal("close wrote to gate twice")
	}
	if err := c.Arm(context.Background()); !errors.Is(err, ErrPTTClosed) {
		t.Fatalf("arm after close: %v", err)
	}
}


// blockingArmPTT simulates an off/receive command that stalls until its
// context is cancelled. The final release made by Close is allowed to finish.
type blockingArmPTT struct {
	mu       sync.Mutex
	calls    int
	entered  chan struct{}
	returned chan struct{}
}

func (p *blockingArmPTT) SetTransmit(ctx context.Context, enabled bool) error {
	if enabled {
		return errors.New("unexpected PTT assertion")
	}
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if call == 1 {
		close(p.entered)
		<-ctx.Done()
		close(p.returned)
		return ctx.Err()
	}
	return nil
}

func TestPTTCloseCancelsBlockedArm(t *testing.T) {
	g := &blockingArmPTT{
		entered:  make(chan struct{}),
		returned: make(chan struct{}),
	}
	c, err := NewPTTController(g, &fakeAudio{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	armDone := make(chan error, 1)
	go func() { armDone <- c.Arm(context.Background()) }()
	select {
	case <-g.entered:
	case <-time.After(time.Second):
		t.Fatal("Arm did not enter the initial off command")
	}
	closed := make(chan error, 1)
	go func() { closed <- c.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close hung waiting for blocked Arm")
	}
	select {
	case <-g.returned:
	default:
		t.Fatal("Close finished before the blocked Arm returned")
	}
	if err := <-armDone; !errors.Is(err, ErrPTTClosed) {
		t.Fatalf("Arm returned %v; want ErrPTTClosed", err)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.calls != 2 {
		t.Fatalf("expected canceled Arm and final receive release, got %d calls", g.calls)
	}
	if err := c.Transmit(context.Background(), "after close"); !errors.Is(err, ErrPTTClosed) {
		t.Fatalf("transmission after close: %v", err)
	}
}
