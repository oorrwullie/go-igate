package main

import (
	"github.com/go-test/deep"
	"github.com/pd0mz/go-aprs"
	"net/textproto"
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
	testSymbol := []byte("[/")
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
		{
			name: "test packet with data",
			fields: fields{
				id:        "1234",
				conn:      &textproto.Conn{},
				connected: true,
				cfg:       Config{},
			},
			args: args{raw: "APRS: AA7AA-7>T0QPSQ,KF6RAL-1*,KF7JAR-10*,WIDE2:`'AXl7K[/`\"C5}_3^[[0m"},
			want: aprs.Packet{
				Raw: "AA7AA-7>T0QPSQ,KF6RAL-1*,KF7JAR-10*,WIDE2:`'AXl7K[/`\"C5}_3^[[0m",
				Src: &aprs.Address{
					Call:     "AA7AA",
					SSID:     7,
					Repeated: false,
				},
				Dst: &aprs.Address{
					Call:     "T0QPSQ",
					SSID:     0,
					Repeated: false,
				},
				Path: aprs.Path{
					&aprs.Address{
						Call:     "KF6RAL",
						SSID:     1,
						Repeated: true,
					},
					&aprs.Address{
						Call:     "KF7JAR",
						SSID:     10,
						Repeated: true,
					},
					&aprs.Address{
						Call:     "WIDE2",
						SSID:     0,
						Repeated: false,
					},
				},
				Payload: "`'AXl7K[/`\"C5}_3^[[0m",
				Position: &aprs.Position{
					Latitude:   40.17183333333333,
					Longitude:  -111.62666666666667,
					Ambiguity:  0,
					Symbol:     aprs.Symbol{},
					Compressed: false,
				},
				Symbol:  aprs.Symbol(testSymbol),
				Comment: "In Service (4km/h, 48°)",
			},
			wantErr: false,
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
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePacket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			diff := deep.Equal(got, tt.want)
			if diff != nil {
				t.Errorf("ParsePacket() got = %v, want %v\ndiff = %v", got, tt.want, diff)
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
