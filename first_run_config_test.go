package main

import (
 "os"
 "strings"
 "testing"

 "github.com/oorrwullie/sparky/internal/config"
)

// A fresh station must not initialize APRS-IS without credentials. The
// first-run template is deliberately RX-only even after a callsign is added.
func TestFirstRunConfigDoesNotEnableNetworkGateOrRFTransmission(t *testing.T) {
 data,err:=os.ReadFile("config.yml.example")
 if err!=nil{t.Fatal(err)}
 sample:=strings.Replace(string(data),`station-callsign: ""`,`station-callsign: "N0CALL-7"`,1)
 if sample==string(data){t.Fatal("sample station callsign placeholder missing")}
 cfg,err:=config.ParseConfigBytes([]byte(sample))
 if err!=nil{t.Fatal(err)}
 if cfg.IGate.Enabled || cfg.IGate.Aprsis.Enabled {
  t.Fatal("first-run sample must not initialize iGate/APRS-IS without a passcode")
 }
 if cfg.Transmitter.Enabled || cfg.DigipeaterEnabled || cfg.IGate.Beacon.Enabled {
  t.Fatal("first-run sample must not transmit, digipeat or beacon")
 }
 if cfg.IGate.Aprsis.Passcode!="" {t.Fatal("first-run sample must not contain a passcode")}
}
