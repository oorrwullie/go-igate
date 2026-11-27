package observability

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// Reception tracks extracted APRS input before duplicate suppression.
type Reception struct {
	received atomic.Uint64
	suppressed atomic.Uint64
	last atomic.Int64
}
func (r *Reception) Received(now time.Time) {
	r.received.Add(1)
	r.last.Store(now.Unix())
}
func (r *Reception) Suppressed() { r.suppressed.Add(1) }
func (r *Reception) Snapshot() RX {
	return RX{r.received.Load(), r.suppressed.Load(), r.last.Load()}
}
type RX struct {
	Received, Suppressed uint64
	Last int64
}
type Operations struct {
	RX RX
	APRSISConfigured, APRSISConnected bool
	APRSISForwarded, APRSISFailed uint64
	TXConfigured bool
	TXDispatched uint64
	ADSBConfigured bool
	ADSBReceived, ADSBPublished, ADSBFailed, ADSBRejected, ADSBOffline, ADSBDropped uint64
	MQTTExternal, MQTTConnected bool
	DecoderConfigured, DecoderRunning bool
}
func (o Operations) writeMetrics(b *strings.Builder) {
	counter := func(name, help string, value uint64) {
		fmt.Fprintf(b, "# HELP sparky_%s %s\n# TYPE sparky_%s counter\nsparky_%s %d\n", name, help, name, name, value)
	}
	gauge := func(name, help string, value int64) {
		fmt.Fprintf(b, "# HELP sparky_%s %s\n# TYPE sparky_%s gauge\nsparky_%s %d\n", name, help, name, name, value)
	}
	flag := func(name, help string, value bool) {
		var v int64
		if value { v = 1 }
		gauge(name, help, v)
	}
	counter("aprs_input_frames_total", "Extracted APRS input before deduplication.", o.RX.Received)
	counter("aprs_duplicates_suppressed_total", "APRS input suppressed by deduplication.", o.RX.Suppressed)
	gauge("aprs_last_input_timestamp_seconds", "Unix time of last extracted APRS input; zero means none.", o.RX.Last)
	if o.APRSISConfigured {
		flag("aprsis_connected", "APRS-IS local verified connection state.", o.APRSISConnected)
		counter("aprsis_rf_forward_writes_total", "RF forward writes completed without local error.", o.APRSISForwarded)
		counter("aprsis_rf_forward_errors_total", "RF forward writes returning an error.", o.APRSISFailed)
	}
	if o.TXConfigured { counter("tx_dispatched_frames_total", "Frames dispatched to backend; not confirmed RF.", o.TXDispatched) }
	if o.ADSBConfigured {
		counter("adsb_input_reports_total", "Reports on configured ADS-B input topic.", o.ADSBReceived)
		counter("adsb_publish_completed_total", "MQTT publish completions; not remote receipt.", o.ADSBPublished)
		counter("adsb_publish_errors_total", "MQTT publish errors or timeouts.", o.ADSBFailed)
		counter("adsb_rejected_total", "Reports rejected by mode validation.", o.ADSBRejected)
		counter("adsb_offline_dropped_total", "Reports dropped while publisher offline.", o.ADSBOffline)
		counter("adsb_queue_dropped_total", "Reports dropped due to full queue.", o.ADSBDropped)
	}
	if o.MQTTExternal { flag("mqtt_input_connected", "External MQTT transport connected.", o.MQTTConnected) }
	if o.DecoderConfigured { flag("decoder_running", "Local decoder reader active; not RF health.", o.DecoderRunning) }
}
