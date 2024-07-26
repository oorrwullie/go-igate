package aprs

import (
	"github.com/go-test/deep"
	"net/textproto"
	"testing"

	"github.com/oorrwullie/go-igate/internal/config"
)

func TestAprsIs_ParsePacket(t *testing.T) {
	type fields struct {
		id        string
		conn      *textproto.Conn
		connected bool
		cfg       config.AprsIs
	}
	type args struct {
		raw string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *Packet
		wantErr bool
	}{
		{
			name: "test empty packet",
			fields: fields{
				id:        "",
				conn:      nil,
				connected: false,
				cfg:       config.AprsIs{},
			},
			args:    args{},
			want:    nil,
			wantErr: true,
		},
		{
			name: "test packet with data",
			fields: fields{
				id:        "1234",
				conn:      &textproto.Conn{},
				connected: true,
				cfg:       config.AprsIs{},
			},
			args: args{raw: "APRS: AA7AA-7>T0QPSQ,KF6RAL-1*,KF7JAR-10*,WIDE2:`'AXl7K[/`\"C5}_3^[[0m"},
			want: &Packet{
				Payload: "`'AXl7K[/`\"C5}_3^[[0m",
				Src:     "APRS: AA7AA-7",
				Dst:     "T0QPSQ",
				Path: []string{
					"KF6RAL-1*",
					"KF7JAR-10*",
					"WIDE2",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePacket(tt.args.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePacket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got == tt.want && tt.want == nil {
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
