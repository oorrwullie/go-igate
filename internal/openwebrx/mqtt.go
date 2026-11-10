// Package openwebrx receives decoded APRS frames published by OpenWebRX+.
package openwebrx

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/cache"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/pubsub"
)

type MQTT struct {
	client mqtt.Client
}

// New connects to an MQTT broker and subscribes to OpenWebRX+'s APRS reports.
func New(cfg config.OpenWebRXMQTT, ps *pubsub.PubSub, packetCache *cache.Cache, logger *log.Logger) (*MQTT, error) {
	if strings.TrimSpace(cfg.Broker) == "" {
		return nil, fmt.Errorf("openwebrx-mqtt.broker is required when OpenWebRX+ MQTT input is enabled")
	}
	topic := strings.TrimSpace(cfg.Topic)
	if topic == "" {
		topic = "openwebrx/APRS"
	}
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(fmt.Sprintf("go-igate-%d", time.Now().UnixNano())).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			logger.Warn("OpenWebRX+ MQTT connection lost: ", err)
		}).
		SetOnConnectHandler(func(client mqtt.Client) {
			token := client.Subscribe(topic, 0, func(_ mqtt.Client, message mqtt.Message) {
				frame, err := extractFrame(message.Payload())
				if err != nil {
					logger.Debug("Ignoring non-APRS OpenWebRX+ MQTT report: ", err)
					return
				}
				if packetCache.Set(frame, time.Now()) {
					logger.Debug("Suppressing duplicate OpenWebRX+ APRS frame: ", frame)
					return
				}
				logger.Debug("OpenWebRX+ APRS frame: ", frame)
				logger.Info("packet received: APRS: ", frame)
				ps.Publish(frame)
			})
			if token.Wait() && token.Error() != nil {
				logger.Error("Could not subscribe to OpenWebRX+ MQTT topic ", topic, ": ", token.Error())
				return
			}
			logger.Info("Subscribed to OpenWebRX+ APRS MQTT topic: ", topic)
		})
	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(15 * time.Second) {
		client.Disconnect(250)
		return nil, fmt.Errorf("timed out connecting to OpenWebRX+ MQTT broker %q", cfg.Broker)
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("connecting to OpenWebRX+ MQTT broker %q: %w", cfg.Broker, err)
	}
	return &MQTT{client: client}, nil
}

func (m *MQTT) Stop() {
	if m != nil && m.client != nil && m.client.IsConnected() {
		m.client.Disconnect(250)
	}
}

func extractFrame(payload []byte) (string, error) {
	text := strings.TrimSpace(string(payload))
	if text == "" {
		return "", fmt.Errorf("empty report")
	}
	if strings.HasPrefix(text, "{") {
		var report struct {
			Mode   string `json:"mode"`
			Msg    string `json:"msg"`
			Packet string `json:"packet"`
			Raw    string `json:"raw"`
			Frame  string `json:"frame"`
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
			text = report.Raw
		}
		if text == "" {
			text = report.Frame
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
