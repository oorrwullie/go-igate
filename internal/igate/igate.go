package igate

import (
	"fmt"
	"strings"
	"sync"
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
		lastRxMu  sync.Mutex
		lastRx    time.Time
	}
)

const minPacketSize = 35
const (
	beaconChannelQuiet  = 3 * time.Second
	beaconMaxWait       = 45 * time.Second
	beaconRetryInterval = 5 * time.Second
)

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
		lastRx:    time.Now(),
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
			i.markRx()

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
	const minInterval = 10 * time.Minute

	rfInterval := i.cfg.Beacon.RFInterval
	isInterval := i.cfg.Beacon.ISInterval

	if rfInterval <= 0 && isInterval <= 0 && i.cfg.Beacon.Interval > 0 {
		rfInterval = i.cfg.Beacon.Interval
		isInterval = i.cfg.Beacon.Interval
	}

	if rfInterval > 0 && rfInterval < minInterval {
		return fmt.Errorf("rf-interval cannot be < 10m")
	}

	if isInterval > 0 && isInterval < minInterval {
		return fmt.Errorf("is-interval cannot be < 10m")
	}

	if rfInterval <= 0 && isInterval <= 0 {
		return fmt.Errorf("beacon interval not configured")
	}

	if i.callSign == "" {
		return fmt.Errorf("beacon call-sign not configured")
	}

	if !i.cfg.Beacon.Enabled {
		fmt.Println("beacon is disabled")
		return nil
	}

	switch {
	case rfInterval > 0 && isInterval > 0:
		i.logger.Info("Starting beacon schedule RF every ", rfInterval, " and APRS-IS every ", isInterval)
	case rfInterval > 0:
		i.logger.Info("Starting beacon schedule RF every ", rfInterval)
	case isInterval > 0:
		i.logger.Info("Starting beacon schedule APRS-IS every ", isInterval)
	}

	var (
		rfTicker *time.Ticker
		isTicker *time.Ticker
		rfChan   <-chan time.Time
		isChan   <-chan time.Time
	)

	if rfInterval > 0 {
		rfTicker = time.NewTicker(rfInterval)
		rfChan = rfTicker.C
	}

	if isInterval > 0 {
		isTicker = time.NewTicker(isInterval)
		isChan = isTicker.C
	}

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
				deadline := time.Now().Add(beaconMaxWait)

				for {
					if i.channelQuietFor(beaconChannelQuiet) {
						time.Sleep(warmup)
						i.tx.Send(msg)
						return
					}

					if time.Now().After(deadline) {
						i.logger.Warn("Skipping beacon -> RF due to continuous channel activity")
						return
					}

					time.Sleep(beaconRetryInterval)
				}
			}(rfFrame)
		}
	}

	// Send initial beacon to each configured destination
	sendBeacon(isInterval > 0, rfInterval > 0)

	go func() {
		defer func() {
			if isTicker != nil {
				isTicker.Stop()
			}

			if rfTicker != nil {
				rfTicker.Stop()
			}
		}()

		for {
			select {
			case <-i.stop:
				return
			case <-isChan:
				sendBeacon(true, false)
			case <-rfChan:
				sendBeacon(false, true)
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

	callSign = strings.ToUpper(strings.TrimSpace(callSign))

	builder.WriteString(packet.Src)
	builder.WriteString(">")
	builder.WriteString(packet.Dst)

	path := append([]string{}, packet.Path...)
	if !containsAprsIsHop(path) && callSign != "" {
		path = append(path, "TCPIP*", "qAR", callSign)
	}

	if len(path) > 0 {
		builder.WriteString(",")
		builder.WriteString(strings.Join(path, ","))
	}

	builder.WriteString(":")
	builder.WriteString(packet.Payload)

	return builder.String()
}

func containsAprsIsHop(path []string) bool {
	for _, component := range path {
		c := strings.ToUpper(strings.TrimSpace(component))
		if strings.HasPrefix(c, "TCPIP") || strings.HasPrefix(c, "TCPXX") || strings.HasPrefix(c, "Q") {
			return true
		}
	}

	return false
}

func (i *IGate) markRx() {
	i.lastRxMu.Lock()
	i.lastRx = time.Now()
	i.lastRxMu.Unlock()
}

func (i *IGate) channelQuietFor(duration time.Duration) bool {
	i.lastRxMu.Lock()
	defer i.lastRxMu.Unlock()

	if i.lastRx.IsZero() {
		return true
	}

	return time.Since(i.lastRx) >= duration
}
