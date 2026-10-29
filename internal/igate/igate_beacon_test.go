package igate

import (
	"testing"
	"time"

	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/transmitter"
)

func TestWaitForQuietUsesQuietDeadlineNotRetryInterval(t *testing.T) {
	oldQuiet := beaconChannelQuiet
	oldRetry := beaconRetryInterval
	defer func() {
		beaconChannelQuiet = oldQuiet
		beaconRetryInterval = oldRetry
	}()
	beaconChannelQuiet = 25 * time.Millisecond
	beaconRetryInterval = 5 * time.Second

	ig := &IGate{
		lastRx: time.Now(),
		stop:   make(chan struct{}),
	}
	started := time.Now()
	if !ig.waitForQuiet(beaconChannelQuiet) {
		t.Fatal("waitForQuiet() returned false")
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("waitForQuiet() took %s; expected it to wait only for the quiet interval", elapsed)
	}
}

func TestSendRFDoesNotWaitForBeaconQuiet(t *testing.T) {
	oldQuiet := beaconChannelQuiet
	defer func() { beaconChannelQuiet = oldQuiet }()
	beaconChannelQuiet = 5 * time.Second

	tx := &transmitter.Tx{Chan: make(chan string, 1)}
	ig := &IGate{
		tx:     tx,
		stop:   make(chan struct{}),
		lastRx: time.Now(),
	}
	done := make(chan bool, 1)
	go func() { done <- ig.SendRF("N0CALL>APRS,WIDE1-1*:test") }()

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("SendRF() returned false")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("digipeater RF send waited for the beacon quiet interval")
	}
	select {
	case <-tx.Chan:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("digipeater RF frame was not queued")
	}
}

func TestSendRFWhenQuietRechecksAfterWaitingForTransmitter(t *testing.T) {
	oldQuiet := beaconChannelQuiet
	defer func() { beaconChannelQuiet = oldQuiet }()
	beaconChannelQuiet = 80 * time.Millisecond

	tx := &transmitter.Tx{Chan: make(chan string, 1)}
	ig := &IGate{
		tx:     tx,
		stop:   make(chan struct{}),
		lastRx: time.Now().Add(-time.Second),
	}

	// Hold transmitter admission so a reception can make the earlier quiet
	// result stale before the beacon is admitted.
	ig.rfTxMu.Lock()
	done := make(chan bool, 1)
	go func() { done <- ig.sendRFFramesWhenQuiet([]string{"N0CALL>APRS:beacon"}) }()
	time.Sleep(20 * time.Millisecond)
	ig.lastRxMu.Lock()
	ig.lastRx = time.Now()
	ig.lastRxMu.Unlock()
	unlockAt := time.Now()
	ig.rfTxMu.Unlock()

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("sendRFFramesWhenQuiet() returned false")
		}
	case <-time.After(time.Second):
		t.Fatal("beacon was not admitted after the fresh quiet interval")
	}
	if elapsed := time.Since(unlockAt); elapsed < beaconChannelQuiet-10*time.Millisecond {
		t.Fatalf("beacon was admitted after %s; expected a fresh quiet interval", elapsed)
	}
	select {
	case <-tx.Chan:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("beacon frame was not queued")
	}
}

func TestSendRFWhenQuietRechecksWhenTransmitterQueueIsFull(t *testing.T) {
	oldQuiet := beaconChannelQuiet
	defer func() { beaconChannelQuiet = oldQuiet }()
	beaconChannelQuiet = 80 * time.Millisecond

	tx := &transmitter.Tx{Chan: make(chan string, 1)}
	tx.Chan <- "already queued"
	ig := &IGate{
		tx:     tx,
		stop:   make(chan struct{}),
		lastRx: time.Now().Add(-time.Second),
	}
	done := make(chan bool, 1)
	go func() { done <- ig.sendRFFramesWhenQuiet([]string{"N0CALL>APRS:beacon"}) }()

	// Let the sender reach the full queue, then make its earlier quiet result
	// stale before freeing the slot.
	time.Sleep(20 * time.Millisecond)
	ig.markRx()
	<-tx.Chan
	unlockAt := time.Now()

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("sendRFFramesWhenQuiet() returned false")
		}
	case <-time.After(time.Second):
		t.Fatal("beacon was not admitted after the fresh quiet interval")
	}
	if elapsed := time.Since(unlockAt); elapsed < beaconChannelQuiet-10*time.Millisecond {
		t.Fatalf("beacon was admitted after %s; expected a fresh quiet interval after queue space became available", elapsed)
	}
	select {
	case got := <-tx.Chan:
		if got != "N0CALL>APRS:beacon" {
			t.Fatalf("unexpected queued frame %q", got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("beacon frame was not queued")
	}
}

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
		ig.sendBeaconRf("N0CALL-1>APRS:TestBeacon", "TestBeacon", true)
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
		ig.sendBeaconRf("N0CALL-5>APRS:Success", "Success", true)
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
		ig.sendBeaconRf("N0CALL-7>APRS:SelfAware", "SelfAware", true)
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
		ig.sendBeaconRf("N0CALL-9>APRS:Beacon", "Beacon", true)
		close(directDone)
	}()

	wideDone := make(chan struct{})
	go func() {
		ig.sendBeaconRf("N0CALL-9>APRS,WIDE1-1,WIDE2-1:Beacon", "Beacon", true)
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

// func TestSendBeaconRfForcesAprsIsAfterMaxAttempts(t *testing.T) {
//     restore := setTestBeaconTimings()
//     defer restore()
//
//     logger, err := log.New()
//     if err != nil {
//         t.Fatalf("failed to create logger: %v", err)
//     }
//
//     tx := &transmitter.Tx{
//         Chan: make(chan string, 4),
//     }
//
//     forced := make(chan string, 1)
//
//     ig := &IGate{
//         cfg: config.IGate{
//             Beacon: config.Beacon{
//                 Comment: "Test",
//                 ISPath:  "TCPIP*,qAR,N0CALL-1",
//             },
//         },
//         callSign:        "N0CALL-1",
//         enableTx:        true,
//         tx:              tx,
//         logger:          logger,
//         stop:            make(chan struct{}),
//         forwardChan:     make(chan *aprs.Packet, 1),
//         maxRfAttempts:   2,
//         disableISBeacon: true,
//         aprsisUpload: func(frame string) error {
//             select {
//             case forced <- frame:
//             default:
//             }
//             return nil
//         },
//     }
//
//     done := make(chan struct{})
//     go func() {
//         ig.sendBeaconRf("N0CALL-1>APRS:Test", "Test", true)
//         close(done)
//     }()
//
//     first := waitForTx(t, tx, 200*time.Millisecond)
//     if first == "" {
//         t.Fatalf("expected first beacon transmission")
//     }
//
//     ig.markRx()
//     ig.observeBeacon(&aprs.Packet{
//         Src:     "OTHERCALL",
//         Payload: "Test",
//     })
//
//     second := waitForTx(t, tx, 200*time.Millisecond)
//     if second == "" || second != first {
//         t.Fatalf("expected second attempt after collision")
//     }
//
//     ig.markRx()
//     ig.observeBeacon(&aprs.Packet{
//         Src:     "OTHERCALL",
//         Payload: "Test",
//     })
//
//     select {
//     case <-done:
//     case <-time.After(300 * time.Millisecond):
//         t.Fatalf("sendBeaconRf did not finish after exhausting attempts")
//     }
//
//     select {
//     case frame := <-forced:
//         if !strings.Contains(frame, "N0CALL-1>APRS") {
//             t.Fatalf("unexpected forced frame %q", frame)
//         }
//     case <-time.After(500 * time.Millisecond):
//         t.Fatalf("expected forced APRS-IS upload after failed attempts")
//     }
//
//     close(ig.stop)
// }

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
