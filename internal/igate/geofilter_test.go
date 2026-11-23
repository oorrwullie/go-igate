package igate

import (
	"testing"

	"github.com/oorrwullie/sparky/internal/aprs"
)

func TestAllowForwardRangePolicy(t *testing.T) {
	inside := &aprs.Packet{Payload: "!4010.20N/11137.80W-Test"}
	outside := &aprs.Packet{Payload: "!4903.50N/07201.75W-Test"}
	noPosition := &aprs.Packet{Payload: ":N0CALL   :hello"}
	ambiguousOutside := &aprs.Packet{Payload: "@121234z4903.5 N/07201.7 W>"}
	compressedInside := &aprs.Packet{Payload: "!/:21D2:OF>  !Test"}
	compressedOutside := &aprs.Packet{Payload: "!/5`=k<;>x>  !Test"}
	timestampedOutside := &aprs.Packet{Payload: "@211200z4903.50N/07201.75W-Test"}

	tests := []struct {
		name   string
		filter *geoFilter
		packet *aprs.Packet
		want   bool
	}{
		{name: "no filter", packet: outside, want: true},
		{name: "inside range", filter: &geoFilter{latCenter: 40.17, lonCenter: -111.63, radiusKm: 100}, packet: inside, want: true},
		{name: "outside range", filter: &geoFilter{latCenter: 40.17, lonCenter: -111.63, radiusKm: 100}, packet: outside, want: false},
		{name: "compressed inside range", filter: &geoFilter{latCenter: 40.17, lonCenter: -111.63, radiusKm: 100}, packet: compressedInside, want: true},
		{name: "compressed outside range", filter: &geoFilter{latCenter: 40.17, lonCenter: -111.63, radiusKm: 100}, packet: compressedOutside, want: false},
		{name: "timestamped outside range", filter: &geoFilter{latCenter: 40.17, lonCenter: -111.63, radiusKm: 100}, packet: timestampedOutside, want: false},
		{name: "ambiguous area entirely out of range", filter: &geoFilter{latCenter: 40.17, lonCenter: -111.63, radiusKm: 100}, packet: ambiguousOutside, want: false},
		{name: "ambiguous area overlaps range", filter: &geoFilter{latCenter: 49.058, lonCenter: -72.028, radiusKm: 0.05}, packet: ambiguousOutside, want: true},
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
