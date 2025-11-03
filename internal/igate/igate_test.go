package igate

import (
	"testing"
	"time"

	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/transmitter"
)

func TestIGate_startBeacon(t *testing.T) {
	type fields struct {
		cfg       config.IGate
		callSign  string
		inputChan chan string
		enableTx  bool
		tx        *transmitter.Tx
		logger    *log.Logger
		Aprsis    *aprs.AprsIs
		stop      chan struct{}
	}
	tests := []struct {
		name          string
		fields        fields
		wantErr       bool
		wantErrString string
	}{
		{
			name: "test short RF beacon interval",
			fields: fields{
				cfg: config.IGate{
					Beacon: config.Beacon{
						Enabled:  true,
						Interval: 1,
					},
				},
				callSign:  "N0CALL-10",
				inputChan: make(chan string),
				enableTx:  false,
				tx:        &transmitter.Tx{},
				Aprsis:    &aprs.AprsIs{},
				stop:      make(chan struct{}),
			},
			wantErr:       true,
			wantErrString: "rf-interval cannot be < 10m",
		},
		{
			name: "test no callsign",
			fields: fields{
				cfg: config.IGate{
					Beacon: config.Beacon{
						Enabled:  true,
						Interval: 10 * time.Minute,
					},
				},
				inputChan: make(chan string),
				enableTx:  false,
				tx:        &transmitter.Tx{},
				Aprsis:    &aprs.AprsIs{},
				stop:      make(chan struct{}),
			},
			wantErr:       true,
			wantErrString: "beacon call-sign not configured",
		},
		{
			name: "disable aprs-is beacons allows short interval",
			fields: fields{
				cfg: config.IGate{
					Beacon: config.Beacon{
						Enabled:    true,
						RFInterval: 30 * time.Minute,
						ISInterval: time.Minute,
						DisableTCP: true,
						DisableRF:  false,
						Comment:    "Test",
						RFPath:     "WIDE1-1",
						ISPath:     "TCPIP*",
					},
					Aprsis: config.AprsIs{Enabled: true},
				},
				callSign:  "N0CALL-10",
				inputChan: make(chan string),
				enableTx:  true,
				tx:        &transmitter.Tx{},
				logger:    mustLogger(t),
				Aprsis:    &aprs.AprsIs{},
				stop:      make(chan struct{}),
			},
			wantErr: false,
		},
		{
			name: "disable both destinations skips beaconing",
			fields: fields{
				cfg: config.IGate{
					Beacon: config.Beacon{
						Enabled:    true,
						RFInterval: 30 * time.Minute,
						ISInterval: 30 * time.Minute,
						DisableRF:  true,
						DisableTCP: true,
					},
				},
				callSign:  "N0CALL-10",
				inputChan: make(chan string),
				enableTx:  true,
				tx:        &transmitter.Tx{},
				logger:    mustLogger(t),
				Aprsis:    &aprs.AprsIs{},
				stop:      make(chan struct{}),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := &IGate{
				cfg:       tt.fields.cfg,
				callSign:  tt.fields.callSign,
				inputChan: tt.fields.inputChan,
				enableTx:  tt.fields.enableTx,
				tx:        tt.fields.tx,
				logger:    tt.fields.logger,
				Aprsis:    tt.fields.Aprsis,
				stop:      tt.fields.stop,
			}
			var err error
			if err = i.startBeacon(); (err != nil) != tt.wantErr {
				t.Errorf("startBeacon() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err.Error() != tt.wantErrString {
				t.Errorf("startBeacon() errorString = %v, wantErrString %v", err.Error(), tt.wantErrString)
			}
			if !tt.wantErr && i.stop != nil {
				close(i.stop)
			}
		})
	}
}

func TestListenForMessagesSkipsSelfForward(t *testing.T) {
	logger := mustLogger(t)
	input := make(chan string, 1)
	forward := make(chan *aprs.Packet, 1)
	stop := make(chan struct{})

	ig := &IGate{
		callSign:    "N0CALL-1",
		inputChan:   input,
		forwardChan: forward,
		stop:        stop,
		logger:      logger,
	}

	done := make(chan struct{})
	go func() {
		_ = ig.listenForMessages()
		close(done)
	}()

	frame := "N0CALL-1>APRS,WIDE1-1:=4010.30N/11137.60W#STATUS TEST"
	input <- frame

	select {
	case pkt := <-forward:
		t.Fatalf("expected no forwarded packet, got %#v", pkt)
	case <-time.After(10 * time.Millisecond):
	}

	close(stop)

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatalf("listenForMessages did not stop")
	}
}

func mustLogger(t *testing.T) *log.Logger {
	t.Helper()

	logger, err := log.New()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	return logger
}
