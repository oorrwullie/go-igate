package aprs

import "testing"

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
