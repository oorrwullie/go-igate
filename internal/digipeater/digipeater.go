package digipeater

import (
	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/pubsub"
	"github.com/oorrwullie/go-igate/internal/transmitter"
)

type Digipeater struct {
	tx        *transmitter.Tx
	InputChan <-chan string
	callsign  string
	logger    *log.Logger
	stop      chan bool
}

const minPacketSize = 35

func New(tx *transmitter.Tx, ps *pubsub.PubSub, callsign string, logger *log.Logger) *Digipeater {
	return &Digipeater{
		tx:        tx,
		InputChan: ps.Subscribe(),
		callsign:  callsign,
		logger:    logger,
		stop:      make(chan bool),
	}
}

func (d *Digipeater) Run() error {
	d.logger.Info("Starting digipeater...")

	for {
		select {
		case <-d.stop:
			return nil
		case msg := <-d.InputChan:
			d.HandleMessage(msg)
		}
	}
}

func (d *Digipeater) Stop() {
	d.logger.Info("Stopping digipeater...")
	d.stop <- true
}

func (d *Digipeater) HandleMessage(msg string) {

	if len(msg) < minPacketSize {
		d.logger.Error("Packet too short: ", msg)
		return
	}

	packet, err := aprs.ParsePacket(msg)
	if err != nil {
		d.logger.Error(err, "Could not parse APRS packet: ", msg)
		return
	}

	needsToBeTransmitted, err := packet.CheckForRetransmit(d.callsign)
	if err != nil {
		d.logger.Error("Error checking for retransmit: ", err)
		return
	}

	if needsToBeTransmitted {
		d.tx.Send(msg)
		d.logger.Info("Message retransmitted: ", msg)
	}
}
