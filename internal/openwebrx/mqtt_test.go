package openwebrx

import (
	"encoding/json"
	"testing"
)

func TestExtractFrameFromOpenWebRXStructuredAPRSReport(t *testing.T) {
	report := struct {
		Source      string   `json:"source"`
		Destination string   `json:"destination"`
		Path        []string `json:"path"`
		Data        string   `json:"data"`
		Mode        string   `json:"mode"`
	}{
		Source:      "N7RIX-7",
		Destination: "T0QPSP",
		Path:        []string{"WIDE1-1", "WIDE2-1"},
		Data:        "`'AYl#/[/`\"CE}_3\r",
		Mode:        "APRS",
	}
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	got, err := extractFrame(payload)
	if err != nil {
		t.Fatalf("extractFrame() error = %v", err)
	}
	want := "N7RIX-7>T0QPSP,WIDE1-1,WIDE2-1:`'AYl#/[/`\"CE}_3"
	if got != want {
		t.Fatalf("extractFrame() = %q, want %q", got, want)
	}
}
