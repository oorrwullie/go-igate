// Package openwebrx receives decoded APRS frames published by OpenWebRX+.
package openwebrx

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
	"github.com/oorrwullie/sparky/internal/aprs"
	"github.com/oorrwullie/sparky/internal/cache"
	"github.com/oorrwullie/sparky/internal/config"
	"github.com/oorrwullie/sparky/internal/log"
	"github.com/oorrwullie/sparky/internal/observability"
	"github.com/oorrwullie/sparky/internal/pubsub"
)

type MQTT struct {
	client           mqtt.Client
	server           *mochi.Server
	mapFlightsClient mapFlightsPublisher
	mapFlightsMu     sync.RWMutex
	mapFlights       config.MapFlights
	aprsTopic        string
	ps               *pubsub.PubSub
	packetCache      *cache.Cache
	logger           *log.Logger
	aprsEvents       chan []byte
	adsbEvents       chan []byte
	stop             chan struct{}
	stopOnce         sync.Once
	workers          sync.WaitGroup
	adsbStats        adsbForwardStats
	Reception observability.Reception
	inputConnected atomic.Bool
	externalInput bool
}

func (m *MQTT) setMapFlightsClient(client mapFlightsPublisher) {
	m.mapFlightsMu.Lock()
	m.mapFlightsClient = client
	m.mapFlightsMu.Unlock()
}

func (m *MQTT) getMapFlightsClient() mapFlightsPublisher {
	m.mapFlightsMu.RLock()
	client := m.mapFlightsClient
	m.mapFlightsMu.RUnlock()
	return client
}

type mapFlightsPublisher interface {
	IsConnectionOpen() bool
	Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token
	Disconnect(quiesce uint)
}

type adsbForwardStats struct {
	received       atomic.Uint64
	published      atomic.Uint64
	publishFailed  atomic.Uint64
	rejected       atomic.Uint64
	offlineDropped atomic.Uint64
	dropped        atomic.Uint64
	lastReceivedAt atomic.Int64
}

// New connects to an MQTT broker and subscribes to OpenWebRX+'s APRS reports.
func New(cfg config.OpenWebRXMQTT, mapFlights config.MapFlights, ps *pubsub.PubSub, packetCache *cache.Cache, logger *log.Logger) (*MQTT, error) {
	topic := strings.TrimSpace(cfg.Topic)
	if topic == "" {
		topic = "openwebrx/APRS"
	}
	m := newEventProcessor(topic, mapFlights, ps, packetCache, logger)
	m.startMapFlightsClient()
	if cfg.EmbeddedBroker.Enabled {
		return newEmbeddedBroker(cfg, m)
	}
	if strings.TrimSpace(cfg.Broker) == "" {
		m.Stop()
		return nil, fmt.Errorf("openwebrx-mqtt.broker is required when OpenWebRX+ MQTT input is enabled")
	}
	m.externalInput = true
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(fmt.Sprintf("sparky-%d", time.Now().UnixNano())).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			m.inputConnected.Store(false)
			logger.Warn("OpenWebRX+ MQTT connection lost: ", err)
		}).
		SetOnConnectHandler(m.subscriptionHandler())
	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(15 * time.Second) {
		client.Disconnect(250)
		m.Stop()
		return nil, fmt.Errorf("timed out connecting to OpenWebRX+ MQTT broker %q", cfg.Broker)
	}
	if err := token.Error(); err != nil {
		client.Disconnect(250)
		m.Stop()
		return nil, fmt.Errorf("connecting to OpenWebRX+ MQTT broker %q: %w", cfg.Broker, err)
	}
	m.client = client
	return m, nil
}

// Paho reuses the client on reconnect. Coalesce notifications while a
// subscription is pending, then subscribe again on the current connection.
func (m *MQTT) subscriptionHandler() mqtt.OnConnectHandler {
	connected := make(chan mqtt.Client, 1)
	topics := map[string]byte{m.aprsTopic: 0}
	if m.mapFlights.Enabled && !mqttTopicMatches(m.aprsTopic, m.mapFlights.Topic) {
		topics[m.mapFlights.Topic] = 0
	}
	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		for {
			select {
			case <-m.stop:
				return
			case client := <-connected:
				token := client.SubscribeMultiple(topics, m.handleMQTTMessage)
				select {
				case <-m.stop:
					return
				case <-token.Done():
					if err := token.Error(); err != nil {
						m.logger.Error("OpenWebRX+ MQTT subscription failed: ", err)
					} else {
						m.logger.Info("Subscribed to OpenWebRX+ MQTT topics: ", topics)
					}
				}
			}
		}
	}()
	return func(c mqtt.Client) {
		m.inputConnected.Store(true)
		select {
		case connected <- c:
		default:
		}
	}
}

func (m *MQTT) handleMQTTMessage(_ mqtt.Client, message mqtt.Message) {
	m.enqueue(message.Topic(), message.Payload())
}

func newEmbeddedBroker(cfg config.OpenWebRXMQTT, m *MQTT) (*MQTT, error) {
	listenAddress := strings.TrimSpace(cfg.EmbeddedBroker.Listen)
	if listenAddress == "" {
		listenAddress = "127.0.0.1:1884"
	}

	server := mochi.New(&mochi.Options{InlineClient: true})
	if err := server.AddHook(new(auth.AllowHook), nil); err != nil {
		m.Stop()
		return nil, fmt.Errorf("adding embedded MQTT allow hook: %w", err)
	}
	if err := server.AddListener(listeners.NewTCP(listeners.Config{ID: "openwebrx", Address: listenAddress})); err != nil {
		m.Stop()
		return nil, fmt.Errorf("listening for OpenWebRX+ MQTT on %s: %w", listenAddress, err)
	}

	if err := server.Subscribe("#", 1, func(_ *mochi.Client, _ packets.Subscription, packet packets.Packet) {
		m.enqueue(packet.TopicName, packet.Payload)
	}); err != nil {
		_ = server.Close()
		m.Stop()
		return nil, fmt.Errorf("subscribing embedded MQTT broker to topics: %w", err)
	}

	go func() {
		if err := server.Serve(); err != nil {
			m.logger.Error("Embedded OpenWebRX+ MQTT broker stopped: ", err)
		}
	}()
	m.server = server
	m.logger.Info("Embedded OpenWebRX+ MQTT broker listening on ", listenAddress, "; APRS topic ", m.aprsTopic)
	return m, nil
}

func mqttTopicMatches(filter, topic string) bool {
	filterParts := strings.Split(filter, "/")
	topicParts := strings.Split(topic, "/")
	for index, part := range filterParts {
		if part == "#" {
			return index == len(filterParts)-1
		}
		if index >= len(topicParts) || (part != "+" && part != topicParts[index]) {
			return false
		}
	}
	return len(filterParts) == len(topicParts)
}

func (m *MQTT) Stop() {
	if m != nil && m.client != nil {
		m.client.Disconnect(250)
		m.inputConnected.Store(false)
	}
	if m != nil && m.server != nil {
		_ = m.server.Close()
	}
	if m != nil {
		m.stopWorkers()
	}
	if m != nil {
		if client := m.getMapFlightsClient(); client != nil {
			client.Disconnect(250)
		}
	}
}

func extractFrame(payload []byte) (string, error) {
	text := strings.TrimSpace(string(payload))
	if text == "" {
		return "", fmt.Errorf("empty report")
	}
	if strings.HasPrefix(text, "{") {
		var report struct {
			Mode        string   `json:"mode"`
			Msg         string   `json:"msg"`
			Packet      string   `json:"packet"`
			Raw         string   `json:"raw"`
			Frame       string   `json:"frame"`
			Source      string   `json:"source"`
			Destination string   `json:"destination"`
			Path        []string `json:"path"`
			Data        string   `json:"data"`
		}
		if err := json.Unmarshal(payload, &report); err != nil {
			return "", fmt.Errorf("invalid JSON report: %w", err)
		}
		if report.Mode != "" && !strings.EqualFold(report.Mode, "APRS") {
			return "", fmt.Errorf("report mode %q is not APRS", report.Mode)
		}
		text = report.Msg
		if text == "" {
			text = report.Packet
		}
		if text == "" {
			text = report.Frame
		}
		if text == "" && report.Source != "" && report.Destination != "" && report.Data != "" {
			// OpenWebRX+ publishes APRS as decoded fields rather than a TNC2 line.
			// Reassemble the packet so Sparky can process it normally.
			path := make([]string, 0, len(report.Path)+1)
			path = append(path, report.Destination)
			for _, component := range report.Path {
				component = strings.TrimSpace(component)
				if component == "" || strings.ContainsAny(component, ">,:\r\n \t") {
					return "", fmt.Errorf("invalid APRS path component %q", component)
				}
				path = append(path, component)
			}
			text = report.Source + ">" + strings.Join(path, ",") + ":" + report.Data
		}
		if text == "" {
			text = report.Raw
		}
	}
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "APRS: "))
	packet, err := aprs.ParsePacket(text)
	if err != nil {
		return "", fmt.Errorf("report does not contain a TNC2 APRS frame: %w", err)
	}
	if strings.ContainsAny(packet.Src, " \t\r\n") || strings.ContainsAny(packet.Dst, " \t\r\n") {
		return "", fmt.Errorf("invalid APRS source or destination")
	}
	return text, nil
}

func (m *MQTT) Operations() observability.Operations {
	return observability.Operations{
		RX: m.Reception.Snapshot(),
		MQTTExternal: m.externalInput, MQTTConnected: m.inputConnected.Load(),
		ADSBConfigured: m.mapFlights.Enabled,
		ADSBReceived: m.adsbStats.received.Load(),
		ADSBPublished: m.adsbStats.published.Load(),
		ADSBFailed: m.adsbStats.publishFailed.Load(),
		ADSBRejected: m.adsbStats.rejected.Load(),
		ADSBOffline: m.adsbStats.offlineDropped.Load(),
		ADSBDropped: m.adsbStats.dropped.Load(),
	}
}
