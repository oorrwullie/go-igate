package main

import (
	"fmt"
	"github.com/pd0mz/go-aprs"
	"net/textproto"
	"reflect"
	"testing"
)

func TestAprsIs_ParsePacket(t *testing.T) {
	type fields struct {
		id        string
		conn      *textproto.Conn
		connected bool
		cfg       Config
	}
	type args struct {
		raw string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    aprs.Packet
		wantErr bool
	}{
		{
			name: "test empty packet",
			fields: fields{
				id:        "",
				conn:      nil,
				connected: false,
				cfg:       Config{},
			},
			args:    args{},
			want:    aprs.Packet{},
			wantErr: true,
		},
		{
			name: "test packet with no data",
			fields: fields{
				id:        "1234",
				conn:      &textproto.Conn{},
				connected: true,
				cfg:       Config{},
			},
			args:    args{raw: "APRS: 1234"},
			want:    aprs.Packet{Raw: "1234"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AprsIs{
				id:        tt.fields.id,
				conn:      tt.fields.conn,
				connected: tt.fields.connected,
				cfg:       tt.fields.cfg,
			}
			got, err := a.ParsePacket(tt.args.raw)
			fmt.Printf("%#v\n", got)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePacket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParsePacket() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isReadReceipt(t *testing.T) {
	type args struct {
		message string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "test aprsc",
			args: args{message: "# aprsc"},
			want: true,
		},
		{
			name: "test javAPRSSrvr",
			args: args{message: "# javAPRSSrvr"},
			want: true,
		},
		{
			name: "test other",
			args: args{message: "other"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isReadReceipt(tt.args.message); got != tt.want {
				t.Errorf("isReadReceipt() = %v, want %v", got, tt.want)
			}
		})
	}
}
