package capture

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeAX25Address(t *testing.T) {
	got, err := encodeAX25Address("APRS", false)
	if err != nil {
		t.Fatalf("encodeAX25Address returned error: %v", err)
	}

	want := []byte{0x82, 0xA0, 0xA4, 0xA6, 0x40, 0x40, 0x60}
	if !bytes.Equal(got, want) {
		t.Fatalf("encodeAX25Address(APRS) = %v, want %v", got, want)
	}
}

func TestEncodeAX25AddressWithSSIDAndRepeated(t *testing.T) {
	got, err := encodeAX25Address("WIDE2-2*", true)
	if err != nil {
		t.Fatalf("encodeAX25Address returned error: %v", err)
	}

	want := []byte{0xAE, 0x92, 0x88, 0x8A, 0x64, 0x40, 0xE5}
	if !bytes.Equal(got, want) {
		t.Fatalf("encodeAX25Address(WIDE2-2*) = %v, want %v", got, want)
	}
}

func TestAprsToAx25Frame(t *testing.T) {
	sc := &SoundcardCapture{}
	const packet = "N0CALL-1>APRS,WIDE1-1:/123456h4903.50N/07201.75W-Test message"

	frame, err := sc.aprsToAx25(packet)
	if err != nil {
		t.Fatalf("aprsToAx25 returned error: %v", err)
	}

	if len(frame) < 7*3+2+2 {
		t.Fatalf("frame too short: %d bytes", len(frame))
	}

	wantDst := []byte{0x82, 0xA0, 0xA4, 0xA6, 0x40, 0x40, 0x60}
	if !bytes.Equal(frame[0:7], wantDst) {
		t.Fatalf("destination field mismatch: %v", frame[0:7])
	}

	wantSrc := []byte{0x9C, 0x60, 0x86, 0x82, 0x98, 0x98, 0x62}
	if !bytes.Equal(frame[7:14], wantSrc) {
		t.Fatalf("source field mismatch: %v", frame[7:14])
	}

	wantPath := []byte{0xAE, 0x92, 0x88, 0x8A, 0x62, 0x40, 0x63}
	if !bytes.Equal(frame[14:21], wantPath) {
		t.Fatalf("path field mismatch: %v", frame[14:21])
	}

	if frame[21] != 0x03 || frame[22] != 0xF0 {
		t.Fatalf("unexpected control/pid bytes: %v %v", frame[21], frame[22])
	}

	payload := string(frame[23 : len(frame)-2])
	if payload != "/123456h4903.50N/07201.75W-Test message" {
		t.Fatalf("payload mismatch: %q", payload)
	}

	gotCRC := binary.LittleEndian.Uint16(frame[len(frame)-2:])
	wantCRC := computeFCS(frame[:len(frame)-2])
	if gotCRC != wantCRC {
		t.Fatalf("CRC mismatch: got 0x%04X want 0x%04X", gotCRC, wantCRC)
	}
}
