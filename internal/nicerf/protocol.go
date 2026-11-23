// Package nicerf implements the documented ASCII command protocol shared by
// NiceRF SA818S and SA868S modules. It does not control GPIO or RF transmission.
package nicerf

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Model string

const (
	SA818S Model = "sa818s-v"
	SA868S Model = "sa868s-v"
)

// Group configures the VHF module without CTCSS/DCS. Frequencies are in Hz,
// avoiding floating-point rounding of the four-decimal MHz wire format.
// SA818S uses the first group field for bandwidth; SA868S uses it for power.
// No unrequested high-power mode is configured for SA868S.
type Group struct {
	TXHz     int64
	RXHz     int64
	Squelch  uint8 // 0 enables monitor mode
	Wideband bool  // SA818S only; 25 kHz when true, otherwise 12.5 kHz
}

func (g Group) Command(model Model) (string, error) {
	tx, err := freq(g.TXHz)
	if err != nil {
		return "", fmt.Errorf("TX frequency: %w", err)
	}
	rx, err := freq(g.RXHz)
	if err != nil {
		return "", fmt.Errorf("RX frequency: %w", err)
	}
	if g.Squelch > 8 {
		return "", fmt.Errorf("squelch %d outside 0..8", g.Squelch)
	}
	var first int
	switch model {
	case SA818S:
		if g.Wideband {
			first = 1
		}
	case SA868S:
		if g.Wideband {
			return "", errors.New("SA868S group command has no bandwidth field")
		}
		first = 1 // documented low-power selector; 0 would request high power
	default:
		return "", fmt.Errorf("unsupported NiceRF model %q", model)
	}
	return fmt.Sprintf("AT+DMOSETGROUP=%d,%s,%s,0000,%d,0000\r\n", first, tx, rx, g.Squelch), nil
}

func freq(hz int64) (string, error) {
	if hz < 134000000 || hz > 174000000 {
		return "", fmt.Errorf("%d Hz outside VHF range", hz)
	}
	if hz%100 != 0 {
		return "", fmt.Errorf("%d Hz cannot be represented in 0.0001 MHz steps", hz)
	}
	return fmt.Sprintf("%d.%04d", hz/1000000, (hz%1000000)/100), nil
}

func Handshake() string { return "AT+DMOCONNECT\r\n" }

// FiltersOff disables emphasis, highpass and lowpass in the module's audio path.
// This is a bench-test setting, not a guarantee of AFSK compatibility.
func FiltersOff() string { return "AT+SETFILTER=1,1,1\r\n" }

// ValidateResponse accepts only the response to the requested command.
// The documentation defines zero as success for these command families.
func ValidateResponse(command, response string) error {
	var expected string
	switch {
	case command == Handshake():
		expected = "+DMOCONNECT"
	case strings.HasPrefix(command, "AT+DMOSETGROUP=") && strings.HasSuffix(command, "\r\n"):
		expected = "+DMOSETGROUP"
	case command == FiltersOff():
		expected = "+DMOSETFILTER"
	default:
		return errors.New("unsupported command")
	}
	s := strings.TrimSuffix(response, "\r\n")
	if s == response {
		return errors.New("response missing CRLF terminator")
	}
	prefix := expected + ":"
	if !strings.HasPrefix(s, prefix) {
		return fmt.Errorf("unexpected response %q (expected %s)", s, expected)
	}
	status := strings.TrimPrefix(s, prefix)
	if len(status) != 1 {
		return fmt.Errorf("invalid status %q", status)
	}
	code, err := strconv.Atoi(status)
	if err != nil {
		return fmt.Errorf("invalid status %q", status)
	}
	if code != 0 {
		return fmt.Errorf("module rejected %s: status %d", expected, code)
	}
	return nil
}
