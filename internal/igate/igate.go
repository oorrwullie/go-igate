package igate

import (
	"fmt"
	"time"

	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
)

type (
	IGate struct {
		cfg       config.IGate
		callSign  string
		inputChan chan string
		enableTx  bool
		txChan    chan string
		logger    *log.Logger
		Aprsis    *aprs.AprsIs
		stop      chan bool
	}
)

const minPacketSize = 35

func New(cfg config.IGate, inputChan chan string, enableTx bool, txChan chan string, callSign string, logger *log.Logger) (*IGate, error) {
	aprsis, err := aprs.New(cfg.Aprsis, callSign, cfg.Beacon.Comment, logger)
	if err != nil {
		return nil, fmt.Errorf("Error creating APRS client: %v", err)
	}

	ig := &IGate{
		cfg:       cfg,
		callSign:  callSign,
		inputChan: inputChan,
		enableTx:  enableTx,
		txChan:    txChan,
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
				fmt.Printf("uploading APRS-IS packet: %v\n", msg)
				err = i.Aprsis.Upload(msg)
				if err != nil {
					i.logger.Error("Error uploading APRS packet: ", err)
					continue
				}

				if i.enableTx && packet.Type().NeedsAck() {
					ackMsg, err := packet.AckString()
					if err != nil {
						i.logger.Error("Error creating APRS acknowledgement message: ", err)
						continue
					}

					i.txChan <- ackMsg
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

	go func() {
		for {
			select {
			case <-ticker.C:
				b := fmt.Sprintf("%s>BEACON:%s", i.callSign, i.cfg.Beacon.Comment)
				i.logger.Info(b)
				i.Aprsis.Conn.PrintfLine(b)
			case <-i.stop:
				ticker.Stop()
				return
			}
		}
	}()

	return nil
}
