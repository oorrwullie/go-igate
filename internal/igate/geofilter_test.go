package igate

import (
	"testing"

	"github.com/oorrwullie/sparky/internal/aprs"
)

func TestAllowForwardRangePolicy(t *testing.T) {
	inside := &aprs.Packet{Payload: "!4010.20N/11137.80W-Test"}
	outside := &aprs.Packet{Payload: "!4903.50N/07201.75W-Test"}
	noPosition := &aprs.Packet{Payload: ":N0CALL   :hello"}

	tests := []struct {
		name   string
		filter *geoFilter
		packet *aprs.Packet
		want   bool
	}{
		{name: "no filter", packet: outside, want: true},
		{name: "inside range", filter: &geoFilter{latCenter: 40.17, lonCenter: -111.63, radiusKm: 100}, packet: inside, want: true},
		{name: "outside range", filter: &geoFilter{latCenter: 40.17, lonCenter: -111.63, radiusKm: 100}, packet: outside, want: false},
		{name: "positionless packet bypasses range", filter: &geoFilter{latCenter: 40.17, lonCenter: -111.63, radiusKm: 100}, packet: noPosition, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ig := &IGate{geoFilter: tt.filter}
			if got := ig.allowForward(tt.packet); got != tt.want {
				t.Fatalf("allowForward() = %t, want %t", got, tt.want)
			}
		})
	}
}
