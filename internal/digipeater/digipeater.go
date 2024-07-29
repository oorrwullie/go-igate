package digipeater

import (
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/transmitter"
)

type Digipeater struct {
	Transmitter *transmitter.Transmitter
	Logger      *log.Logger
}

func New(tx *transmitter.Transmitter, logger *log.Logger) *Digipeater {
	return &Digipeater{
		Transmitter: tx,
		Logger:      logger,
	}
}

func (d *Digipeater) Start() error {
	d.Logger.Info("Starting digipeater...")
	return nil
}

func (d *Digipeater) Stop() {
	d.Logger.Info("Stopping digipeater...")
}

func (d *Digipeater) HandleMessage(msg string) {
	d.Logger.Info("Handling message: ", msg)
	// Implement digigate rules here
	d.Logger.Info("Message retransmitted: ", msg)
}
