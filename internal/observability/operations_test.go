package observability

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOperationalMetrics(t *testing.T) {
	var rx Reception
	rx.Received(time.Unix(123, 0))
	rx.Received(time.Unix(124, 0))
	rx.Suppressed()
	r := New()
	r.OperationalSnapshot = func() Operations {
		return Operations{RX: rx.Snapshot(), APRSISConfigured:true, APRSISConnected:true, APRSISForwarded:3, APRSISFailed:1, TXConfigured:true, TXDispatched:2, ADSBConfigured:true, ADSBReceived:5}
	}
	w := httptest.NewRecorder()
	r.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	for _, want := range []string{"sparky_aprs_input_frames_total 2\n", "sparky_aprs_duplicates_suppressed_total 1\n", "sparky_aprs_last_input_timestamp_seconds 124\n", "sparky_aprsis_connected 1\n", "sparky_aprsis_rf_forward_writes_total 3\n", "sparky_aprsis_rf_forward_errors_total 1\n", "sparky_tx_dispatched_frames_total 2\n", "sparky_adsb_input_reports_total 5\n"} {
		if !strings.Contains(w.Body.String(),want) { t.Errorf("missing %q",want) }
	}
	if strings.Contains(w.Body.String(),"sparky_mqtt_input_connected") { t.Fatal("unconfigured input exported") }
}

func BenchmarkReceptionRecord(b *testing.B) {
	var rx Reception
	now := time.Unix(123,0)
	b.ReportAllocs()
	for i:=0; i<b.N; i++ { rx.Received(now) }
}
