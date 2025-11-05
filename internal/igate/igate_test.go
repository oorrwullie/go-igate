package igate

import (
	"sync"
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
			name: "additional rf beacon interval too short",
			fields: fields{
				cfg: config.IGate{
					Beacon: config.Beacon{
						Enabled: true,
						ExtraRF: []config.RFBeacon{
							{
								Path:     "",
								Interval: 5 * time.Minute,
							},
						},
					},
				},
				callSign:  "N0CALL-10",
				inputChan: make(chan string),
				enableTx:  false,
				tx:        &transmitter.Tx{},
				logger:    mustLogger(t),
				Aprsis:    &aprs.AprsIs{},
				stop:      make(chan struct{}),
			},
			wantErr:       true,
			wantErrString: "additional-rf-beacons interval cannot be < 10m",
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
				enableTx:  false,
				tx:        &transmitter.Tx{},
				logger:    mustLogger(t),
				Aprsis:    &aprs.AprsIs{},
				stop:      make(chan struct{}),
			},
			wantErr: false,
		},
		{
			name: "additional rf beacon without primary",
			fields: fields{
				cfg: config.IGate{
					Beacon: config.Beacon{
						Enabled: true,
						ExtraRF: []config.RFBeacon{
							{
								Path:     "",
								Interval: 10 * time.Minute,
							},
						},
						DisableTCP: true,
					},
				},
				callSign:  "N0CALL-10",
				inputChan: make(chan string),
				enableTx:  false,
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

func TestSendBeaconRfAssumesSuccessAfterTimeout(t *testing.T) {
	logger := mustLogger(t)

	// Preserve global timing knobs used by sendBeaconRf.
	oldCollision := beaconCollisionWindow
	oldWarmup := beaconWarmup
	oldRetry := beaconRetryInterval
	oldQuiet := beaconChannelQuiet
	defer func() {
		beaconCollisionWindow = oldCollision
		beaconWarmup = oldWarmup
		beaconRetryInterval = oldRetry
		beaconChannelQuiet = oldQuiet
	}()

	beaconCollisionWindow = 10 * time.Millisecond
	beaconWarmup = 0
	beaconRetryInterval = time.Millisecond
	beaconChannelQuiet = 0

	tx := &transmitter.Tx{
		Chan: make(chan string, 1),
	}

	ig := &IGate{
		cfg: config.IGate{
			Beacon: config.Beacon{},
		},
		enableTx: true,
		tx:       tx,
		logger:   logger,
		stop:     make(chan struct{}),
	}
	ig.lastRx = time.Now().Add(-time.Minute)

	done := make(chan struct{})
	go func() {
		ig.sendBeaconRf("N0CALL>APRS:Test", "Test")
		close(done)
	}()

	select {
	case <-tx.Chan:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("expected RF beacon transmission")
	}

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("sendBeaconRf did not return after timeout")
	}

	close(ig.stop)
}

func TestStartBeaconSequencesAprsIsBeforeRf(t *testing.T) {
	logger := mustLogger(t)

	oldCollision := beaconCollisionWindow
	oldWarmup := beaconWarmup
	oldRetry := beaconRetryInterval
	oldQuiet := beaconChannelQuiet
	defer func() {
		beaconCollisionWindow = oldCollision
		beaconWarmup = oldWarmup
		beaconRetryInterval = oldRetry
		beaconChannelQuiet = oldQuiet
	}()

	beaconCollisionWindow = 10 * time.Millisecond
	beaconWarmup = 0
	beaconRetryInterval = 1 * time.Millisecond
	beaconChannelQuiet = 0

	aprsCalls := make(chan string, 4)
	var aprsMu sync.Mutex
	var aprsFrames []string

	tx := &transmitter.Tx{
		Chan: make(chan string, 1),
	}

	ig := &IGate{
		cfg: config.IGate{
			Beacon: config.Beacon{
				Enabled:    true,
				RFInterval: 10 * time.Minute,
				ISInterval: 10 * time.Minute,
				DisableRF:  false,
				DisableTCP: false,
				Comment:    "Test Comment",
				RFPath:     "WIDE1-1",
				ISPath:     "TCPIP*",
			},
		},
		callSign: "N0CALL-1",
		enableTx: true,
		tx:       tx,
		logger:   logger,
		stop:     make(chan struct{}),
		aprsisUpload: func(frame string) error {
			aprsMu.Lock()
			aprsFrames = append(aprsFrames, frame)
			aprsMu.Unlock()
			aprsCalls <- frame
			return nil
		},
	}
	ig.lastRx = time.Now().Add(-time.Minute)

	if err := ig.startBeacon(); err != nil {
		t.Fatalf("startBeacon() error = %v", err)
	}

	select {
	case <-aprsCalls:
	case <-time.After(50 * time.Millisecond):
		t.Fatalf("expected APRS-IS beacon before RF transmission")
	}

	select {
	case <-tx.Chan:
	case <-time.After(50 * time.Millisecond):
		t.Fatalf("expected RF beacon transmission")
	}

	aprsMu.Lock()
	if len(aprsFrames) == 0 {
		t.Fatalf("expected APRS-IS frame when both destinations enabled")
	}
	aprsMu.Unlock()

	close(ig.stop)
	time.Sleep(10 * time.Millisecond)
}

func mustLogger(t *testing.T) *log.Logger {
	t.Helper()

	logger, err := log.New()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	return logger
}
