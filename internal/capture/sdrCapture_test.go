package capture

import (
	"bytes"
	"io"
	"log"
	"os/exec"
	"testing"

	"github.com/oorrwullie/go-igate/internal/config"
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
