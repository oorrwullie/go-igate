package igate

import (
	"testing"
	"time"

	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/transmitter"
)

func setTestBeaconTimings() func() {
	origQuiet := beaconChannelQuiet
	origRetry := beaconRetryInterval
	origCollision := beaconCollisionWindow
	origWarmup := beaconWarmup

	beaconChannelQuiet = 1 * time.Millisecond
	beaconRetryInterval = 2 * time.Millisecond
	beaconCollisionWindow = 15 * time.Millisecond
	beaconWarmup = 0

	return func() {
		beaconChannelQuiet = origQuiet
		beaconRetryInterval = origRetry
		beaconCollisionWindow = origCollision
		beaconWarmup = origWarmup
	}
}

func TestSendBeaconRfRetriesOnCollision(t *testing.T) {
	restore := setTestBeaconTimings()
	defer restore()

	logger, err := log.New()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	tx := &transmitter.Tx{
		Chan: make(chan string, 4),
	}

	ig := &IGate{
		callSign:    "N0CALL-1",
		enableTx:    true,
		tx:          tx,
		logger:      logger,
		stop:        make(chan struct{}),
		forwardChan: make(chan *aprs.Packet, 1),
	}

	done := make(chan struct{})
	go func() {
		ig.sendBeaconRf("N0CALL-1>APRS:TestBeacon", "TestBeacon")
		close(done)
	}()

	first := waitForTx(t, tx, 200*time.Millisecond)
	if first == "" {
		t.Fatalf("expected first beacon transmission")
	}

	ig.markRx()
	ig.observeBeacon(&aprs.Packet{
		Src:     "OTHERCALL",
		Payload: "TestBeacon",
	})

	second := waitForTx(t, tx, 200*time.Millisecond)
	if second == "" || second != first {
		t.Fatalf("expected retry after collision, got %q", second)
	}

	ig.markRx()
	ig.observeBeacon(&aprs.Packet{
		Src:     "N0CALL-1",
		Payload: "TestBeacon",
	})

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("sendBeaconRf did not finish after successful retry")
	}

	close(ig.stop)
}

func TestSendBeaconRfSucceedsFirstAttempt(t *testing.T) {
	restore := setTestBeaconTimings()
	defer restore()

	logger, err := log.New()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	tx := &transmitter.Tx{
		Chan: make(chan string, 2),
	}

	ig := &IGate{
		callSign:    "N0CALL-5",
		enableTx:    true,
		tx:          tx,
		logger:      logger,
		stop:        make(chan struct{}),
		forwardChan: make(chan *aprs.Packet, 1),
	}

	done := make(chan struct{})
	go func() {
		ig.sendBeaconRf("N0CALL-5>APRS:Success", "Success")
		close(done)
	}()

	first := waitForTx(t, tx, 200*time.Millisecond)
	if first == "" {
		t.Fatalf("expected beacon transmission")
	}

	ig.markRx()
	ig.observeBeacon(&aprs.Packet{
		Src:     "N0CALL-5",
		Payload: "Success",
	})

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("sendBeaconRf did not terminate after successful acknowledgment")
	}

	select {
	case extra := <-tx.Chan:
		t.Fatalf("unexpected additional transmission: %q", extra)
	default:
	}

	close(ig.stop)
}

func TestSendBeaconRfIgnoresSelfCollision(t *testing.T) {
	restore := setTestBeaconTimings()
	defer restore()

	logger, err := log.New()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	tx := &transmitter.Tx{
		Chan: make(chan string, 4),
	}

	ig := &IGate{
		callSign:    "N0CALL-7",
		enableTx:    true,
		tx:          tx,
		logger:      logger,
		stop:        make(chan struct{}),
		forwardChan: make(chan *aprs.Packet, 1),
	}

	done := make(chan struct{})
	go func() {
		ig.sendBeaconRf("N0CALL-7>APRS:SelfAware", "SelfAware")
		close(done)
	}()

	first := waitForTx(t, tx, 200*time.Millisecond)
	if first == "" {
		t.Fatalf("expected initial beacon transmission")
	}

	ig.markRx()
	ig.observeBeacon(&aprs.Packet{
		Src:     "OTHER-1",
		Path:    []string{"N0CALL-7*", "WIDE2-1"},
		Payload: "SelfAware",
	})

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("sendBeaconRf did not finish after acknowledging self collision via path")
	}

	select {
	case retry := <-tx.Chan:
		t.Fatalf("unexpected retry after self-collision: %q", retry)
	default:
	}

	close(ig.stop)
}

func TestSendBeaconRfHandlesConcurrentSchedules(t *testing.T) {
	restore := setTestBeaconTimings()
	defer restore()

	logger, err := log.New()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	tx := &transmitter.Tx{
		Chan: make(chan string, 4),
	}

	ig := &IGate{
		callSign:    "N0CALL-9",
		enableTx:    true,
		tx:          tx,
		logger:      logger,
		stop:        make(chan struct{}),
		forwardChan: make(chan *aprs.Packet, 1),
	}

	directDone := make(chan struct{})
	go func() {
		ig.sendBeaconRf("N0CALL-9>APRS:Beacon", "Beacon")
		close(directDone)
	}()

	wideDone := make(chan struct{})
	go func() {
		ig.sendBeaconRf("N0CALL-9>APRS,WIDE1-1,WIDE2-1:Beacon", "Beacon")
		close(wideDone)
	}()

	frames := map[string]bool{
		"N0CALL-9>APRS:Beacon":                 false,
		"N0CALL-9>APRS,WIDE1-1,WIDE2-1:Beacon": false,
	}

	ackFrame := func(frame string) {
		switch frame {
		case "N0CALL-9>APRS:Beacon":
			ig.markRx()
			ig.observeBeacon(&aprs.Packet{
				Src:     "N0CALL-9",
				Payload: "Beacon",
			})
		case "N0CALL-9>APRS,WIDE1-1,WIDE2-1:Beacon":
			ig.markRx()
			ig.observeBeacon(&aprs.Packet{
				Src:     "N0CALL-9",
				Path:    []string{"WIDE1-1", "WIDE2-1"},
				Payload: "Beacon",
			})
		default:
			t.Fatalf("unexpected frame value %q", frame)
		}
	}

	first := waitForTx(t, tx, 200*time.Millisecond)
	if _, ok := frames[first]; !ok {
		t.Fatalf("unexpected first frame: %q", first)
	}
	frames[first] = true
	ackFrame(first)

	second := waitForTx(t, tx, 200*time.Millisecond)
	if _, ok := frames[second]; !ok {
		t.Fatalf("unexpected second frame: %q", second)
	}
	frames[second] = true
	ackFrame(second)

	if !frames["N0CALL-9>APRS:Beacon"] || !frames["N0CALL-9>APRS,WIDE1-1,WIDE2-1:Beacon"] {
		t.Fatalf("missing expected frames: %+v", frames)
	}

	select {
	case <-directDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("direct beacon did not finish")
	}

	select {
	case <-wideDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("wide beacon did not finish")
	}

	time.Sleep(10 * time.Millisecond)

	select {
	case extra := <-tx.Chan:
		t.Fatalf("unexpected extra transmission: %q", extra)
	default:
	}

	close(ig.stop)
}

func waitForTx(t *testing.T, tx *transmitter.Tx, timeout time.Duration) string {
	t.Helper()

	select {
	case msg := <-tx.Chan:
		return msg
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for tx output")
		return ""
	}
}
