package capture

import (
	"bytes"
	"io"
	"log"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/oorrwullie/sparky/internal/config"
)

type tReader struct {
	ReadBytes []byte
}

func TestSdr_pipeRtlFM(t *testing.T) {
	type fields struct {
		cfg        config.Sdr
		logger     *log.Logger
		outputChan chan []byte
		Cmd        *exec.Cmd
	}
	type args struct {
		out io.ReadCloser
		buf []byte
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   []byte
	}{
		{
			name: "test basic input",
			fields: fields{
				cfg:        config.Sdr{},
				logger:     log.New(log.Writer(), "", 0),
				outputChan: make(chan []byte),
				Cmd:        exec.Command("rtl_fm"),
			},
			args: args{
				out: &tReader{ReadBytes: make([]byte, 0)},
				buf: []byte("test"),
			},
			want: []byte("test"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SdrCapture{
				cfg:        tt.fields.cfg,
				outputChan: tt.fields.outputChan,
				Cmd:        tt.fields.Cmd,
			}
			go func() {
				o := <-s.outputChan
				if !bytes.Equal(o, tt.want) {
					t.Errorf("pipeRtlFM() = %v, want %v", o, tt.want)
				}
			}()
			s.pipeRtlFM(tt.args.out, tt.args.buf)
		})
	}
}

func (tR *tReader) Read(p []byte) (n int, err error) {
	len := len(p)
	if cap(tR.ReadBytes) == 0 {
		tR.ReadBytes = make([]byte, len)
	}
	if bytes.Equal(tR.ReadBytes, p) {
		return 0, io.EOF
	}
	if len == 0 {
		return 0, io.EOF
	}
	copy(tR.ReadBytes, p[:len])
	return len, nil
}

func (tR *tReader) Close() error {
	tR.ReadBytes = []byte{}
	return nil
}

func TestSdrPipeStopsWhenOutputIsBlocked(t *testing.T) {
	s := &SdrCapture{
		outputChan: make(chan []byte), // no consumer
		stop:       make(chan struct{}),
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.pipeRtlFM(io.NopCloser(strings.NewReader("packet")), make([]byte, 16))
	}()

	select {
	case <-done:
		t.Fatal("capture unexpectedly completed without a consumer")
	case <-time.After(30 * time.Millisecond):
	}

	// A closed stop channel releases a blocked output send.
	s.stopOnce.Do(func() { close(s.stop) })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("capture did not stop while output was blocked")
	}
}

func TestSdrStopWithoutSuccessfulStart(t *testing.T) {
	for _, tc := range []struct {
		name       string
		attemptRun bool
	}{
		{name: "never started"},
		{name: "start failed", attemptRun: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &SdrCapture{
				Cmd:  exec.Command("sparky-nonexistent-rtl-fm-for-test"),
				stop: make(chan struct{}),
				done: make(chan struct{}),
			}
			if tc.attemptRun {
				if err := s.Start(); err == nil {
					t.Fatal("expected rtl_fm startup failure")
				}
			}

			finished := make(chan struct{})
			go func() {
				s.Stop()
				s.Stop() // Idempotent, including the failed-start case.
				close(finished)
			}()
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("Stop blocked without a running rtl_fm process")
			}
			if err := s.Start(); err == nil {
				t.Fatal("Start must not restart a stopped capture")
			}
		})
	}
}
