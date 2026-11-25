package openwebrx

import (
	"errors"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/oorrwullie/sparky/internal/config"
	"github.com/oorrwullie/sparky/internal/cache"
	"github.com/oorrwullie/sparky/internal/pubsub"
	"github.com/oorrwullie/sparky/internal/log"
)

type fakeMapFlightsPublisher struct {
	connected bool
	payload   interface{}
	topic     string
	qos       byte
	tokenErr  error
}

func (f *fakeMapFlightsPublisher) IsConnectionOpen() bool { return f.connected }
func (f *fakeMapFlightsPublisher) Disconnect(uint)        {}
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

type subscriptionClient struct {
	mqtt.Client
	calls chan map[string]byte
	ack   chan struct{}
}

type subscriptionToken struct {
	completedMQTTToken
	ack <-chan struct{}
}

func (t subscriptionToken) Done() <-chan struct{} { return t.ack }

func (c *subscriptionClient) SubscribeMultiple(topics map[string]byte, _ mqtt.MessageHandler) mqtt.Token {
	c.calls <- topics
	return subscriptionToken{ack: c.ack}
}

func TestReconnectWhileSubscriptionPending(t *testing.T) {
	m := newADSBTestProcessor()
	m.aprsTopic = "openwebrx/APRS"
	m.stop = make(chan struct{})
	defer m.stopWorkers()
	c := &subscriptionClient{calls: make(chan map[string]byte, 2), ack: make(chan struct{})}
	onConnect := m.subscriptionHandler()
	checkSubscription := func() {
		t.Helper()
		select {
		case topics := <-c.calls:
			for _, topic := range []string{m.aprsTopic, m.mapFlights.Topic} {
				if qos, ok := topics[topic]; !ok || qos != 0 {
					t.Fatalf("unexpected subscriptions: %v", topics)
				}
			}
		case <-time.After(time.Second):
			t.Fatal("missing subscription")
		}
	}
	onConnect(c)
	checkSubscription()
	returned := make(chan struct{})
	go func() { onConnect(c); close(returned) }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("reconnect callback blocked")
	}
	close(c.ack)
	checkSubscription()
}

func TestADSBWorkersDuringSustainedOutage(t *testing.T) {
	m := newADSBTestProcessor()
	m.stop = make(chan struct{})
	defer m.stopWorkers()
	m.setMapFlightsClient(&fakeMapFlightsPublisher{})
	for range adsbWorkers {
		m.workers.Add(1)
		go m.runADSBWorker()
	}
	const count = 100
	deadline := time.After(2 * time.Second)
	for range count {
		select {
		case m.adsbEvents <- []byte(`{"mode":"ADSB"}`):
		case <-deadline:
			t.Fatal("workers stalled while disconnected")
		}
	}
	m.stopWorkers()
	if got := m.adsbStats.offlineDropped.Load(); got != count {
		t.Fatalf("offline drops = %d, want %d", got, count)
	}
}

func TestOperationalAPRSInput(t *testing.T) {
 m := newADSBTestProcessor()
 m.packetCache = cache.NewCache(10, "", time.Minute)
 m.ps = pubsub.New()
 subscriber := m.ps.Subscribe()
 frame := "N0CALL-7>APRS:>test"
 m.handleAPRS([]byte(frame))
 m.handleAPRS([]byte(frame))
 m.handleAPRS([]byte("invalid"))
 got := m.Operations().RX
 if got.Received != 2 || got.Suppressed != 1 || got.Last <= 0 {
  t.Fatalf("unexpected reception snapshot: %+v", got)
 }
 select {
 case actual := <-subscriber:
  if actual != frame { t.Fatalf("published %q", actual) }
 default:
  t.Fatal("missing first frame")
 }
 select {
 case actual := <-subscriber:
  t.Fatalf("unexpected duplicate delivery %q", actual)
 default:
 }
}

func TestOperationalADSBOutcomes(t *testing.T) {
 m := newADSBTestProcessor()
 payload := []byte(`{"mode":"ADSB"}`)
 m.enqueue("openwebrx/ADSB", payload)
 m.enqueue("openwebrx/ADSB", payload)
 m.setMapFlightsClient(&fakeMapFlightsPublisher{connected:true})
 m.forwardADSB(<-m.adsbEvents)
 m.setMapFlightsClient(&fakeMapFlightsPublisher{connected:true,tokenErr:errors.New("failed")})
 m.forwardADSB(payload)
 m.forwardADSB([]byte(`{"mode":"APRS"}`))
 m.setMapFlightsClient(&fakeMapFlightsPublisher{})
 m.forwardADSB(payload)
 o := m.Operations()
 if !o.ADSBConfigured || o.ADSBReceived != 2 || o.ADSBPublished != 1 || o.ADSBFailed != 1 || o.ADSBRejected != 1 || o.ADSBOffline != 1 || o.ADSBDropped != 1 {
  t.Fatalf("unexpected ADS-B snapshot: %+v", o)
 }
 if m.Operations() != o { t.Fatal("scrape reset counters") }
}
