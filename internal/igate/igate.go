package igate

import (
	"fmt"
	"strings"
	"time"

	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/pubsub"
	"github.com/oorrwullie/go-igate/internal/transmitter"
)

type (
	IGate struct {
		cfg       config.IGate
		callSign  string
		inputChan <-chan string
		enableTx  bool
		tx        *transmitter.Tx
		logger    *log.Logger
		Aprsis    *aprs.AprsIs
		stop      chan bool
	}
)

const minPacketSize = 35

func New(cfg config.IGate, ps *pubsub.PubSub, enableTx bool, tx *transmitter.Tx, callSign string, logger *log.Logger) (*IGate, error) {
	aprsis, err := aprs.New(cfg.Aprsis, callSign, cfg.Beacon.Comment, logger)
	if err != nil {
		return nil, fmt.Errorf("Error creating APRS client: %v", err)
	}

	inputChan := ps.Subscribe()

	ig := &IGate{
		cfg:       cfg,
		callSign:  callSign,
		inputChan: inputChan,
		enableTx:  enableTx,
		tx:        tx,
		logger:    logger,
		stop:      make(chan bool),
		Aprsis:    aprsis,
	}

	return ig, nil
}

func (i *IGate) Run() error {
	if i.cfg.Beacon.Enabled {
		err := i.startBeacon()
		if err != nil {
			return fmt.Errorf("Error starting beacon: %v", err)
		}
	}

	i.listenForMessages()

	return nil
}

func (i *IGate) Stop() {
	i.stop <- true
}

func (i *IGate) listenForMessages() {
	for {
		select {
		case <-i.stop:
			return
		case msg := <-i.inputChan:
			if len(msg) < minPacketSize {
				i.logger.Error("Packet too short: ", msg)
				continue
			}

			packet, err := aprs.ParsePacket(msg)
			if err != nil {
				i.logger.Error(err, "Could not parse APRS packet: ", msg)
				continue
			}

			if !packet.IsAckMessage() && packet.Type().ForwardToAprsIs() {
				uploadFrame := formatForAprsIs(packet, i.callSign)
				fmt.Printf("uploading APRS-IS packet: %v\n", uploadFrame)
				err = i.Aprsis.Upload(uploadFrame)
				if err != nil {
					i.logger.Error("Error uploading APRS packet: ", err)
					continue
				}
			}
		}
	}
}

func (i *IGate) startBeacon() error {
	if i.cfg.Beacon.Interval < (time.Duration(10) * time.Minute) {
		return fmt.Errorf("interval cannot be < 10m")
	}

	if i.callSign == "" {
		return fmt.Errorf("beacon call-sign not configured")
	}

	if !i.cfg.Beacon.Enabled {
		fmt.Println("beacon is disabled")
		return nil
	}

	i.logger.Info("Starting beacon every ", i.cfg.Beacon.Interval)

	ticker := time.NewTicker(i.cfg.Beacon.Interval)

	sendBeacon := func(toAprsIs, toRf bool) {
		if toAprsIs {
			isFrame := buildBeaconFrame(i.callSign, i.cfg.Beacon.ISPath, i.cfg.Beacon.Comment)
			i.logger.Info("Beacon -> APRS-IS: ", isFrame)
			i.Aprsis.Conn.PrintfLine(isFrame)
		}

		if toRf && i.enableTx && i.tx != nil {
			rfFrame := buildBeaconFrame(i.callSign, i.cfg.Beacon.RFPath, i.cfg.Beacon.Comment)
			i.logger.Info("Beacon -> RF: ", rfFrame)
			go func(msg string) {
				const warmup = 2 * time.Second
				time.Sleep(5 * time.Second)
				i.tx.Send(msg)
			}(rfFrame)
		}
	}

	// Send initial beacon to both RF and APRS-IS
	sendBeacon(true, true)

	go func() {
		for {
			select {
			case <-ticker.C:
				// Periodic beacons go to APRS-IS only
				sendBeacon(true, false)
			case <-i.stop:
				ticker.Stop()
				return
			}
		}
	}()

	return nil
}

func buildBeaconFrame(callSign, path, comment string) string {
	path = strings.TrimSpace(path)

	var builder strings.Builder
	builder.WriteString(callSign)
	builder.WriteString(">APRS")

	if path != "" {
		builder.WriteString(",")
		builder.WriteString(path)
	}

	builder.WriteString(":")
	builder.WriteString(comment)

	return builder.String()
}

func formatForAprsIs(packet *aprs.Packet, callSign string) string {
	var builder strings.Builder

	builder.WriteString(packet.Src)
	builder.WriteString(">")
	builder.WriteString(packet.Dst)
	builder.WriteString(",TCPIP*,qAR,")
	builder.WriteString(strings.ToUpper(strings.TrimSpace(callSign)))
	builder.WriteString(":")
	builder.WriteString(packet.Payload)

	return builder.String()
}
