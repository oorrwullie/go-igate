package aprs

import (
	"strings"
	"testing"
)

func TestParsePacketWithThirdPartyPayload(t *testing.T) {
	const raw = "KB7COX-10>APDW17,KF6RAL-1*,WIDE2*:}HK3BCA-7>APWW11,TCPIP,KB7COX-10*::KJ7STI-1 :N:HOTG"

	packet, err := ParsePacket(raw)
	if err != nil {
		t.Fatalf("ParsePacket returned error: %v", err)
	}

	if packet.Src != "KB7COX-10" {
		t.Fatalf("unexpected source callsign: got %q", packet.Src)
	}

	if packet.Dst != "APDW17" {
		t.Fatalf("unexpected destination: got %q", packet.Dst)
	}

	wantPath := []string{"KF6RAL-1*", "WIDE2*"}
	if len(packet.Path) != len(wantPath) {
		t.Fatalf("unexpected path length: got %d want %d", len(packet.Path), len(wantPath))
	}
	for i := range wantPath {
		if packet.Path[i] != wantPath[i] {
			t.Fatalf("unexpected path component at %d: got %q want %q", i, packet.Path[i], wantPath[i])
		}
	}

	wantPayload := "}HK3BCA-7>APWW11,TCPIP,KB7COX-10*::KJ7STI-1 :N:HOTG"
	if packet.Payload != wantPayload {
		t.Fatalf("unexpected payload: got %q want %q", packet.Payload, wantPayload)
	}
}

func TestPacketPosition(t *testing.T) {
	packet, err := ParsePacket("CALL>APRS:!4010.30N/11137.60W#Test")
	if err != nil {
		t.Fatalf("ParsePacket returned error: %v", err)
	}

	lat, lon, ok := packet.Position()
	if !ok {
		t.Fatalf("expected position to be parsed")
	}

	if lat < 40.17 || lat > 40.18 {
		t.Fatalf("unexpected latitude %f", lat)
	}

	if lon > -111.62 || lon < -111.64 {
		t.Fatalf("unexpected longitude %f", lon)
	}

	msgPacket, err := ParsePacket("CALL>APRS:>Status text")
	if err != nil {
		t.Fatalf("ParsePacket returned error: %v", err)
	}

	if _, _, ok := msgPacket.Position(); ok {
		t.Fatalf("expected no position for status report")
	}
}

func TestUnwrapThirdPartyForAprsIs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{
			name: "unwraps-rf-third-party-packet",
			raw:  "IGATE>APRS,WIDE1-1:}CALL>APRS,WIDE1-1*:!4903.50N/07201.75W-Test",
			want: "CALL>APRS,WIDE1-1*:!4903.50N/07201.75W-Test",
			ok:   true,
		},
		{
			name: "rejects-aprs-is-third-party-packet",
			raw:  "IGATE>APRS,WIDE1-1:}CALL>APRS,TCPIP*:!4903.50N/07201.75W-Test",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := ParsePacket(tt.raw)
			if err != nil {
				t.Fatalf("ParsePacket returned error: %v", err)
			}

			got, ok := packet.UnwrapThirdPartyForAprsIs()
			if ok != tt.ok {
				t.Fatalf("unexpected result: got ok=%v want %v", ok, tt.ok)
			}
			if ok && formatPacketForTest(got) != tt.want {
				t.Fatalf("unexpected unwrapped packet: got %q want %q", formatPacketForTest(got), tt.want)
			}
		})
	}
}

func TestHasForbiddenRFPath(t *testing.T) {
	for _, path := range []string{"NOGATE", "RFONLY", "TCPIP*", "TCPXX", "qAR", "qAO"} {
		packet, err := ParsePacket("CALL>APRS," + path + ":!4903.50N/07201.75W-Test")
		if err != nil {
			t.Fatalf("ParsePacket returned error for %q: %v", path, err)
		}
		if !packet.HasForbiddenRFPath() {
			t.Errorf("expected path %q to be forbidden", path)
		}
	}
}

func TestMessageDestination(t *testing.T) {
	packet, err := ParsePacket("CALL>APRS::N0CALL-10:hello{01}")
	if err != nil {
		t.Fatalf("ParsePacket returned error: %v", err)
	}

	if packet.Type() != Message {
		t.Fatalf("expected Message, got %v", packet.Type())
	}

	destination, ok := packet.MessageDestination()
	if !ok || destination != "N0CALL-10" {
		t.Fatalf("unexpected destination: %q, %v", destination, ok)
	}
}

func formatPacketForTest(packet *Packet) string {
	return packet.Src + ">" + packet.Dst + "," + strings.Join(packet.Path, ",") + ":" + packet.Payload
}
