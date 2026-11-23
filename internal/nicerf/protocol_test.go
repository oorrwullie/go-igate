package nicerf

import (
	"strings"
	"testing"
)

func TestGroupCommandModelSpecificFirstField(t *testing.T) {
	g := Group{TXHz: 144390000, RXHz: 144390000, Squelch: 0}
	cases := []struct {
		model Model
		want  string
	}{
		{SA818S, "AT+DMOSETGROUP=0,144.3900,144.3900,0000,0,0000\r\n"},
		{SA868S, "AT+DMOSETGROUP=1,144.3900,144.3900,0000,0,0000\r\n"},
	}
	for _, tc := range cases {
		got, err := g.Command(tc.model)
		if err != nil || got != tc.want {
			t.Fatalf("%s: got %q, %v want %q", tc.model, got, err, tc.want)
		}
	}
	g.Wideband = true
	if got, err := g.Command(SA818S); err != nil || !strings.HasPrefix(got, "AT+DMOSETGROUP=1,") {
		t.Fatalf("wide SA818S %q: %v", got, err)
	}
	if _, err := g.Command(SA868S); err == nil {
		t.Fatal("SA868S must reject SA818S bandwidth option")
	}
}

func TestGroupValidation(t *testing.T) {
	cases := []Group{
		{TXHz: 133999900, RXHz: 144390000},
		{TXHz: 144390050, RXHz: 144390000},
		{TXHz: 144390000, RXHz: 174000100},
		{TXHz: 144390000, RXHz: 144390000, Squelch: 9},
	}
	for _, g := range cases {
		if _, err := g.Command(SA818S); err == nil {
			t.Fatalf("accepted invalid group %+v", g)
		}
	}
	if _, err := (Group{TXHz: 144390000, RXHz: 144390000}).Command(Model("wrong")); err == nil {
		t.Fatal("accepted unknown module")
	}
}

func TestResponses(t *testing.T) {
	cases := []struct {
		cmd, reply string
		ok         bool
	}{
		{Handshake(), "+DMOCONNECT:0\r\n", true},
		{FiltersOff(), "+DMOSETFILTER:0\r\n", true},
		{"AT+DMOSETGROUP=1,144.3900,144.3900,0000,0,0000\r\n", "+DMOSETGROUP:0\r\n", true},
		{Handshake(), "+DMOCONNECT:1\r\n", false},
		{Handshake(), "+DMOSETGROUP:0\r\n", false},
		{Handshake(), "+DMOCONNECT:0", false},
		{Handshake(), "+DMOCONNECT:00\r\n", false},
		{Handshake(), "+DMOCONNECT:x\r\n", false},
		{"AT+UNKNOWN\r\n", "+UNKNOWN:0\r\n", false},
	}
	for _, tc := range cases {
		err := ValidateResponse(tc.cmd, tc.reply)
		if (err == nil) != tc.ok {
			t.Errorf("cmd=%q reply=%q err=%v wanted success=%v", tc.cmd, tc.reply, err, tc.ok)
		}
	}
}
