package openwebrx

import (
	"errors"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
)

type fakeMapFlightsPublisher struct {
	connected bool
	payload   interface{}
	topic     string
	qos       byte
	tokenErr  error
}

func (f *fakeMapFlightsPublisher) IsConnected() bool { return f.connected }
func (f *fakeMapFlightsPublisher) Disconnect(uint)   {}
func (f *fakeMapFlightsPublisher) Publish(topic string, qos byte, _ bool, payload interface{}) mqtt.Token {
	f.topic = topic
	f.qos = qos
	f.payload = payload
	return completedMQTTToken{err: f.tokenErr}
}

type completedMQTTToken struct{ err error }

func (completedMQTTToken) Wait() bool                       { return true }
func (completedMQTTToken) WaitTimeout(_ time.Duration) bool { return true }
func (completedMQTTToken) Done() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}
func (t completedMQTTToken) Error() error { return t.err }

func newADSBTestProcessor() *MQTT {
	logger, _ := log.New()
	return &MQTT{
		mapFlights: config.MapFlights{Enabled: true, Topic: "openwebrx/ADSB"},
		adsbEvents: make(chan []byte, 1),
		logger:     logger,
	}
}

func TestADSBForwardingTracksReceiveAndPublish(t *testing.T) {
	m := newADSBTestProcessor()
	publisher := &fakeMapFlightsPublisher{connected: true}
	m.setMapFlightsClient(publisher)
	payload := []byte(`{"mode":"ADSB","icao":"A18964","lat":40.189957,"lon":-111.693848}`)

	m.enqueue("openwebrx/ADSB", payload)
	if got := m.adsbStats.received.Load(); got != 1 {
		t.Fatalf("received = %d, want 1", got)
	}
	if m.adsbStats.lastReceivedAt.Load() == 0 {
		t.Fatal("last received timestamp was not recorded")
	}
	m.forwardADSB(<-m.adsbEvents)

	if got := m.adsbStats.published.Load(); got != 1 {
		t.Fatalf("published = %d, want 1", got)
	}
	if got := m.adsbStats.publishFailed.Load(); got != 0 {
		t.Fatalf("publish failures = %d, want 0", got)
	}
	if publisher.topic != "openwebrx/ADSB" || publisher.qos != 0 {
		t.Fatalf("published topic/qos = %q/%d, want openwebrx/ADSB/0", publisher.topic, publisher.qos)
	}
	if got, ok := publisher.payload.([]byte); !ok || string(got) != string(payload) {
		t.Fatalf("published payload = %#v, want original report", publisher.payload)
	}
}

func TestADSBForwardingCountsFailuresAndRejectedReports(t *testing.T) {
	m := newADSBTestProcessor()
	m.setMapFlightsClient(&fakeMapFlightsPublisher{connected: true, tokenErr: errors.New("publish failed")})
	m.forwardADSB([]byte(`{"mode":"ADSB","icao":"A18964"}`))
	m.forwardADSB([]byte(`{"mode":"APRS"}`))

	if got := m.adsbStats.publishFailed.Load(); got != 1 {
		t.Fatalf("publish failures = %d, want 1", got)
	}
	if got := m.adsbStats.rejected.Load(); got != 1 {
		t.Fatalf("rejected = %d, want 1", got)
	}
}

func TestADSBQueueDropIsCounted(t *testing.T) {
	m := newADSBTestProcessor()
	m.enqueue("openwebrx/ADSB", []byte(`{"mode":"ADSB"}`))
	m.enqueue("openwebrx/ADSB", []byte(`{"mode":"ADSB"}`))

	if got := m.adsbStats.received.Load(); got != 2 {
		t.Fatalf("received = %d, want 2", got)
	}
	if got := m.adsbStats.dropped.Load(); got != 1 {
		t.Fatalf("dropped = %d, want 1", got)
	}
}
