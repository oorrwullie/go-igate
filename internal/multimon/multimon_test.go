package multimon

import (
	"os/exec"
	"testing"
	"time"
	"github.com/oorrwullie/sparky/internal/log"
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
	if m.Cmd.ProcessState == nil || !m.Cmd.ProcessState.Exited() {
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
