package digipeater

import (
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/transmitter"
)

type Digipeater struct {
	Transmitter *transmitter.Transmitter
	Logger      *log.Logger
}

func NewDigipeater(tx *transmitter.Transmitter, logger *log.Logger) *Digipeater {
	return &Digipeater{
		Transmitter: tx,
		Logger:      logger,
	}
}

func (d *Digipeater) Start() error {
	d.Logger.Info("Starting digipeater...")
	err := d.Transmitter.StartTx()
	if err != nil {
		d.Logger.Error("Failed to start transmitter: ", err)
		return err
	}
	return nil
}

func (d *Digipeater) Stop() {
	d.Logger.Info("Stopping digipeater...")
	d.Transmitter.StopTx()
}

func (d *Digipeater) HandleMessage(msg string) {
	d.Logger.Info("Handling message: ", msg)
	// Implement digigate rules here
	d.Transmitter.Transmit(msg)
	d.Logger.Info("Message retransmitted: ", msg)
}
