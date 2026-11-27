package multimon

import (
	"os/exec"
	"testing"
	"time"
	"github.com/oorrwullie/sparky/internal/log"
	"github.com/oorrwullie/sparky/internal/cache"
	"github.com/oorrwullie/sparky/internal/pubsub"
)

func TestIsSelfMessage(t *testing.T) {
	m := &Multimon{
		stationCallsign: "N7RIX-10",
	}

	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{
			name: "exact match",
			msg:  "N7RIX-10>APRS:payload",
			want: true,
		},
		{
			name: "lowercase source",
			msg:  "n7rix-10>APRS:payload",
			want: true,
		},
		{
			name: "different callsign",
			msg:  "OTHER-1>APRS:payload",
			want: false,
		},
		{
			name: "missing source delimiter",
			msg:  "INVALID FRAME",
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := m.isSelfMessage(tt.msg)
			if got != tt.want {
				t.Fatalf("isSelfMessage(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestNormalizeMultimonMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want string
	}{
		{name: "APRS packet", msg: "APRS: N7RIX-7>T0QPSP:payload", want: "N7RIX-7>T0QPSP:payload"},
		{name: "startup banner", msg: "Enabled demodulators: AFSK1200", want: ""},
		{name: "missing colon", msg: "APRS: N0CALL>APRS", want: ""},
		{name: "missing source", msg: "APRS: >APRS:>status", want: ""},
		{name: "missing destination", msg: "APRS: N0CALL>,WIDE1-1:>status", want: ""},
		{name: "invalid source whitespace", msg: "APRS: N0 CALL>APRS:>status", want: ""},
		{name: "invalid destination whitespace", msg: "APRS: N0CALL>AP RS:>status", want: ""},
		{name: "third party", msg: "APRS: N0CALL>APRS:}OTHER>APRS:>status", want: "N0CALL>APRS:}OTHER>APRS:>status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeMultimonMessage(tt.msg); got != tt.want {
				t.Fatalf("normalizeMultimonMessage(%q) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

func TestMultimonStopReapsChild(t *testing.T) {
	logger, err := log.New()
	if err != nil { t.Fatal(err) }
	m := &Multimon{
		Cmd: exec.Command("sh", "-c", "cat >/dev/null"),
		inputChan: make(chan []byte),
		logger: logger,
		stopCh: make(chan struct{}),
		stopHealth: make(chan struct{}),
	}
	if err := m.Start(); err != nil { t.Fatal(err) }
	process := m.Cmd.Process
	done := make(chan struct{})
	go func() { m.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3*time.Second):
		t.Fatal("Stop did not join child")
	}
	if m.Cmd.ProcessState == nil {
		t.Fatal("child process was not reaped via Wait")
	}
	if err := process.Signal(nil); err == nil {
		t.Fatal("child process still alive after Stop")
	}
	m.Stop()
}

func TestMultimonStopBeforeStart(t *testing.T) {
	logger, err := log.New()
	if err != nil { t.Fatal(err) }
	m := &Multimon{Cmd: exec.Command("sh", "-c", "exit 0"), logger: logger, stopCh: make(chan struct{}), stopHealth: make(chan struct{})}
	m.Stop()
	m.Stop()
	if err := m.Start(); err == nil {
		t.Fatal("Start succeeded after Stop")
	}
}

func TestDecoderReceptionMetrics(t *testing.T) {
 logger, err := log.New()
 if err != nil { t.Fatal(err) }
 ps := pubsub.New()
 subscriber := ps.Subscribe()
 m := &Multimon{
  Cmd: exec.Command("sh", "-c", "printf '%s\\n' 'APRS: N0CALL>APRS' 'APRS: N0CALL>APRS'; read line; printf '%s\\n' 'APRS: N0CALL>APRS:>status' 'APRS: N0CALL>APRS:>status'"),
  inputChan: make(chan []byte, 1), logger: logger,
  cache: cache.NewCache(10, "", time.Minute), pubsub: ps,
  stopCh: make(chan struct{}), stopHealth: make(chan struct{}),
 }
 if err := m.Start(); err != nil { t.Fatal(err) }
 defer m.Stop()
 // First run a malformed-only child separately below, rather than relying
 // on a sleep to establish that the scanner has processed its output.
 m.inputChan <- []byte("continue\n")
 close(m.inputChan)
 select {
 case <-m.done:
 case <-time.After(3*time.Second): t.Fatal("decoder did not finish")
 }
 got := m.Reception.Snapshot()
 if got.Received != 2 || got.Suppressed != 1 || got.Last <= 0 {
  t.Fatalf("unexpected reception snapshot: %+v", got)
 }
 select {
 case frame := <-subscriber:
  if frame != "N0CALL>APRS:>status" { t.Fatalf("published %q", frame) }
 default: t.Fatal("missing valid frame")
 }
 select {
 case frame := <-subscriber: t.Fatalf("unexpected delivery %q", frame)
 default:
 }
 bad := &Multimon{
  Cmd: exec.Command("sh", "-c", "printf '%s\\n' 'APRS: N0CALL>APRS' 'APRS: N0CALL>APRS'"),
  inputChan: make(chan []byte), logger: logger,
  cache: cache.NewCache(10, "", time.Minute), pubsub: ps,
  stopCh: make(chan struct{}), stopHealth: make(chan struct{}),
 }
 if err := bad.Start(); err != nil { t.Fatal(err) }
 defer bad.Stop()
 select {
 case <-bad.done:
 case <-time.After(3*time.Second): t.Fatal("malformed decoder did not finish")
 }
 invalid := bad.Reception.Snapshot()
 if invalid.Received != 0 || invalid.Suppressed != 0 || invalid.Last != 0 {
  t.Fatalf("malformed input altered reception: %+v", invalid)
 }
 select {
 case frame := <-subscriber: t.Fatalf("published malformed frame %q", frame)
 default:
 }
}

func TestNormalizeMultimonMessageNoAllocations(t *testing.T) {
 if n := testing.AllocsPerRun(1000, func(){ if normalizeMultimonMessage("APRS: N0CALL>APRS,WIDE1-1:>status") == "" { panic("valid frame rejected") } }); n != 0 {
  t.Fatalf("normalization allocated %.2f objects", n)
 }
}
