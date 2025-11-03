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
				cfg:       config.IGate{Beacon: config.Beacon{Interval: 1}},
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
				cfg:       config.IGate{Beacon: config.Beacon{Interval: 10 * time.Minute}},
				inputChan: make(chan string),
				enableTx:  false,
				tx:        &transmitter.Tx{},
				Aprsis:    &aprs.AprsIs{},
				stop:      make(chan struct{}),
			},
			wantErr:       true,
			wantErrString: "beacon call-sign not configured",
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
		})
	}
}
