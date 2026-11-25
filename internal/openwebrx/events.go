package openwebrx

import (
	"crypto/tls"
	"encoding/json"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/oorrwullie/sparky/internal/cache"
	"github.com/oorrwullie/sparky/internal/config"
	"github.com/oorrwullie/sparky/internal/log"
	"github.com/oorrwullie/sparky/internal/pubsub"
)

const (
	aprsEventQueueSize   = 256
	adsbEventQueueSize   = 1024
	adsbWorkers          = 4
	adsbDiagnosticPeriod = time.Minute
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
		m.workers.Add(1)
		go m.runADSBHealthLogger()
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
		m.adsbStats.received.Add(1)
		m.adsbStats.lastReceivedAt.Store(time.Now().UnixNano())
		select {
		case <-m.stop:
			return
		case m.adsbEvents <- append([]byte(nil), payload...):
		default:
			m.adsbStats.dropped.Add(1)
			m.logger.Warn("map.flights ADS-B queue is full; dropping report")
		}
	}
}

func (m *MQTT) runADSBHealthLogger() {
	defer m.workers.Done()
	var previous [6]uint64
	ticker := time.NewTicker(adsbDiagnosticPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			lastReceived := "never"
			if timestamp := m.adsbStats.lastReceivedAt.Load(); timestamp != 0 {
				lastReceived = time.Since(time.Unix(0, timestamp)).Round(time.Second).String()
			}
			client := m.getMapFlightsClient()
			connected := client != nil && client.IsConnectionOpen()
			current := [6]uint64{m.adsbStats.received.Load(), m.adsbStats.published.Load(), m.adsbStats.publishFailed.Load(), m.adsbStats.rejected.Load(), m.adsbStats.offlineDropped.Load(), m.adsbStats.dropped.Load()}
			var delta [6]uint64
			for i := range current { delta[i] = current[i] - previous[i] }
			previous = current
			m.logger.Info(
				"ADS-B forwarding health (last 60s): received=", delta[0],
				" mqtt-publish-complete=", delta[1],
				" publish-errors=", delta[2],
				" rejected=", delta[3],
				" offline-dropped=", delta[4],
				" queue-dropped=", delta[5],
				" queue=", len(m.adsbEvents), "/", cap(m.adsbEvents),
				" map-connected=", connected,
				" last-report-age=", lastReceived,
				" (QoS 0; MQTT completion does not confirm map display)",
			)
		case <-m.stop:
			return
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
	now := time.Now()
	m.Reception.Received(now)
	if m.packetCache.Set(frame, now) {
		m.Reception.Suppressed()
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
		m.adsbStats.rejected.Add(1)
		m.logger.Debug("Ignoring non-ADS-B OpenWebRX+ MQTT report on map.flights feed")
		return
	}
	client := m.getMapFlightsClient()
	if client == nil || !client.IsConnectionOpen() {
		m.adsbStats.offlineDropped.Add(1)
		return
	}
	token := client.Publish(m.mapFlights.Topic, 0, false, payload)
	if !token.WaitTimeout(5 * time.Second) {
		m.adsbStats.publishFailed.Add(1)
		m.logger.Warn("Timed out sending ADS-B report to map.flights")
	} else if err := token.Error(); err != nil {
		m.adsbStats.publishFailed.Add(1)
		m.logger.Warn("Could not send ADS-B report to map.flights: ", err)
	} else {
		m.adsbStats.published.Add(1)
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
		SetClientID("sparky-mapflights-" + strings.TrimSpace(m.mapFlights.Station)).
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
	client := mqtt.NewClient(opts)
	m.setMapFlightsClient(client)
	token := client.Connect()
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
