package aprs

import (
	"bufio"
	"net/textproto"
	"strings"
	"testing"
)

func TestVerifyLoginResponseSkipsServerGreeting(t *testing.T) {
	response := textproto.NewReader(bufio.NewReader(strings.NewReader(
		"# aprsc 2.1.20 test server\r\n# logresp N0CALL-10 verified, server test\r\n",
	)))

	if err := verifyLoginResponse(response, "N0CALL-10"); err != nil {
		t.Fatalf("verifyLoginResponse() rejected valid response after greeting: %v", err)
	}
}

func TestVerifyLoginResponseRejectsUnverifiedCallsign(t *testing.T) {
	response := textproto.NewReader(bufio.NewReader(strings.NewReader(
		"# aprsc 2.1.20 test server\r\n# logresp N0CALL-10 unverified, server test\r\n",
	)))

	if err := verifyLoginResponse(response, "N0CALL-10"); err == nil {
		t.Fatal("verifyLoginResponse() accepted an unverified callsign")
	}
}
