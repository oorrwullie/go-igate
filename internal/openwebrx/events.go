package openwebrx

import (
	"crypto/tls"
	"encoding/json"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/oorrwullie/go-igate/internal/cache"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/pubsub"
)

const (
	aprsEventQueueSize = 256
	adsbEventQueueSize = 1024
	adsbWorkers        = 4
)

func newEventProcessor(aprsTopic string, mapCfg config.MapFlights, ps *pubsub.PubSub, packetCache *cache.Cache, logger *log.Logger) *MQTT {
	if strings.TrimSpace(aprsTopic) == "" {
		aprsTopic = "openwebrx/APRS"
	}
	if strings.TrimSpace(mapCfg.Topic) == "" {
		mapCfg.Topic = "openwebrx/ADSB"
	}
	m := &MQTT{
		aprsTopic:   aprsTopic,
		mapFlights:  mapCfg,
		ps:          ps,
		packetCache: packetCache,
		logger:      logger,
		aprsEvents:  make(chan []byte, aprsEventQueueSize),
		adsbEvents:  make(chan []byte, adsbEventQueueSize),
		stop:        make(chan struct{}),
	}
	m.workers.Add(1)
	go m.runAPRSWorker()
	if mapCfg.Enabled {
		for range adsbWorkers {
			m.workers.Add(1)
			go m.runADSBWorker()
		}
	}
	return m
}

func (m *MQTT) enqueue(topic string, payload []byte) {
	if mqttTopicMatches(m.aprsTopic, topic) {
		select {
		case <-m.stop:
			return
		case m.aprsEvents <- append([]byte(nil), payload...):
		default:
			m.logger.Warn("OpenWebRX+ APRS event queue is full; dropping packet")
		}
		return
	}
	if m.mapFlights.Enabled && mqttTopicMatches(m.mapFlights.Topic, topic) {
		select {
		case <-m.stop:
			return
		case m.adsbEvents <- append([]byte(nil), payload...):
		default:
			m.logger.Warn("map.flights ADS-B queue is full; dropping report")
		}
	}
}

func (m *MQTT) runAPRSWorker() {
	defer m.workers.Done()
	for {
		select {
		case payload := <-m.aprsEvents:
			m.handleAPRS(payload)
		case <-m.stop:
			for {
				select {
				case payload := <-m.aprsEvents:
					m.handleAPRS(payload)
				default:
					return
				}
			}
		}
	}
}

func (m *MQTT) handleAPRS(payload []byte) {
	frame, err := extractFrame(payload)
	if err != nil {
		m.logger.Debug("Ignoring non-APRS OpenWebRX+ MQTT report: ", err)
		return
	}
	if m.packetCache.Set(frame, time.Now()) {
		m.logger.Debug("Suppressing duplicate OpenWebRX+ APRS frame: ", frame)
		return
	}
	m.logger.Debug("OpenWebRX+ APRS frame: ", frame)
	m.logger.Info("packet received: APRS: ", frame)
	m.ps.Publish(frame)
}

func (m *MQTT) runADSBWorker() {
	defer m.workers.Done()
	for {
		select {
		case payload := <-m.adsbEvents:
			m.forwardADSB(payload)
		case <-m.stop:
			for {
				select {
				case payload := <-m.adsbEvents:
					m.forwardADSB(payload)
				default:
					return
				}
			}
		}
	}
}

func (m *MQTT) forwardADSB(payload []byte) {
	var report struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(payload, &report); err != nil || !strings.EqualFold(report.Mode, "ADSB") {
		m.logger.Debug("Ignoring non-ADS-B OpenWebRX+ MQTT report on map.flights feed")
		return
	}
	if m.mapFlightsClient == nil {
		return
	}
	for !m.mapFlightsClient.IsConnected() {
		select {
		case <-m.stop:
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
	token := m.mapFlightsClient.Publish(m.mapFlights.Topic, 0, false, payload)
	if !token.WaitTimeout(5 * time.Second) {
		m.logger.Warn("Timed out sending ADS-B report to map.flights")
	} else if err := token.Error(); err != nil {
		m.logger.Warn("Could not send ADS-B report to map.flights: ", err)
	}
}

func (m *MQTT) startMapFlightsClient() {
	if !m.mapFlights.Enabled {
		return
	}
	broker := strings.TrimSpace(m.mapFlights.Broker)
	if broker == "" {
		broker = "ssl://mqtt.map.flights:8883"
	}
	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID("go-igate-mapflights-" + strings.TrimSpace(m.mapFlights.Station)).
		SetUsername(strings.TrimSpace(m.mapFlights.Station)).
		SetPassword(m.mapFlights.Key).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOrderMatters(false).
		SetTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			m.logger.Warn("map.flights MQTT connection lost: ", err)
		}).
		SetOnConnectHandler(func(mqtt.Client) {
			m.logger.Info("Connected to map.flights MQTT as station ", m.mapFlights.Station)
		})
	m.mapFlightsClient = mqtt.NewClient(opts)
	token := m.mapFlightsClient.Connect()
	go func() {
		select {
		case <-token.Done():
			if err := token.Error(); err != nil {
				m.logger.Warn("Could not connect to map.flights MQTT broker ", broker, ": ", err)
			}
		case <-m.stop:
		}
	}()
}

func (m *MQTT) stopWorkers() {
	m.stopOnce.Do(func() { close(m.stop) })
	m.workers.Wait()
}
