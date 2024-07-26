package transmitter

import (
	"fmt"
	"github.com/oorrwullie/go-igate/internal/log"
	"os"
	"testing"
)

func TestTransmitter_Transmit(t1 *testing.T) {
	logger, err := log.New()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	type fields struct {
		TxChan chan string
		Stop   chan bool
		Logger *log.Logger
	}
	type args struct {
		msg string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
	}{
		{
			name: "test no input",
			fields: fields{
				TxChan: make(chan string),
				Stop:   make(chan bool),
				Logger: logger,
			},
			args: args{
				msg: "",
			},
			want: "",
		},
		{
			name: "test with APRS message",
			fields: fields{
				TxChan: make(chan string),
				Stop:   make(chan bool),
				Logger: logger,
			},
			args: args{
				msg: "AA7AA-7>T0QPSQ,KF6RAL-1*,KF7JAR-10*,WIDE2:`'AXl7K[/`\"C5}_3^[[0m",
			},
			want: "AA7AA-7>T0QPSQ,KF6RAL-1*,KF7JAR-10*,WIDE2:`'AXl7K[/`\"C5}_3^[[0m",
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &Transmitter{
				TxChan: tt.fields.TxChan,
				Stop:   tt.fields.Stop,
				Logger: tt.fields.Logger,
			}
			go func() {
				if got := <-tt.fields.TxChan; got != tt.want {
					t1.Errorf("Transmitter.Transmit() = %v, want %v", got, tt.want)
				}
			}()
			t.Transmit(tt.args.msg)
		})
	}
}
