package digipeater

import (
	"testing"
	"time"

	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/pubsub"
	"github.com/oorrwullie/go-igate/internal/transmitter"
)

func newTestDigipeater(t *testing.T) (*Digipeater, *transmitter.Tx) {
	t.Helper()

	tx := &transmitter.Tx{
		Chan: make(chan string, 1),
	}

	ps := pubsub.New()

	logger, err := log.New()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	cfg := config.Digipeater{
		AliasPatterns: []string{`^WIDE1-1$`},
		WidePatterns:  []string{`^WIDE[1-7]-[1-7]$`},
		DedupeWindow:  time.Minute,
	}

	dp, err := New(tx, ps, "N0CALL-1", cfg, logger)
	if err != nil {
		t.Fatalf("failed to create digipeater: %v", err)
	}

	return dp, tx
}

func readTx(t *testing.T, tx *transmitter.Tx) string {
	t.Helper()

	select {
	case msg := <-tx.Chan:
		return msg
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for transmitter output")
		return ""
	}
}

func TestDigipeaterWideChain(t *testing.T) {
	dp, tx := newTestDigipeater(t)

	packet := "CALL1>APRS,WIDE1-1,WIDE2-1:/123456h4903.50N/07201.75W-Test message"
	dp.HandleMessage(packet)

	got := readTx(t, tx)
	want := "CALL1>APRS,N0CALL-1*,WIDE2-1:/123456h4903.50N/07201.75W-Test message"

	if got != want {
		t.Fatalf("unexpected digipeated packet\nwant %q\ngot  %q", want, got)
	}
}

func TestDigipeaterWideHopInsert(t *testing.T) {
	dp, tx := newTestDigipeater(t)

	packet := "CALL1>APRS,WIDE2-2:/123456h4903.50N/07201.75W-Test message"
	dp.HandleMessage(packet)

	got := readTx(t, tx)
	want := "CALL1>APRS,N0CALL-1*,WIDE2-1:/123456h4903.50N/07201.75W-Test message"

	if got != want {
		t.Fatalf("unexpected digipeated packet\nwant %q\ngot  %q", want, got)
	}
}

func TestDigipeaterSuppressDuplicate(t *testing.T) {
	dp, tx := newTestDigipeater(t)

	packet := "CALL1>APRS,WIDE1-1:/123456h4903.50N/07201.75W-Test message"

	dp.HandleMessage(packet)
	_ = readTx(t, tx)

	dp.HandleMessage(packet)

	select {
	case msg := <-tx.Chan:
		t.Fatalf("expected duplicate suppression, but transmitted %q", msg)
	case <-time.After(100 * time.Millisecond):
	}
}
