package digigate

import (
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/transmitter"
)

type Digigate struct {
	Transmitter *transmitter.Transmitter
	Logger      *log.Logger
}

func NewDigigate(tx *transmitter.Transmitter, logger *log.Logger) *Digigate {
	return &Digigate{
		Transmitter: tx,
		Logger:      logger,
	}
}

func (d *Digigate) Start() error {
	d.Logger.Info("Starting digigate...")
	err := d.Transmitter.StartTx()
	if err != nil {
		d.Logger.Error("Failed to start transmitter: ", err)
		return err
	}
	return nil
}

func (d *Digigate) Stop() {
	d.Logger.Info("Stopping digigate...")
	d.Transmitter.StopTx()
}

func (d *Digigate) HandleMessage(msg string) {
	d.Logger.Info("Handling message: ", msg)
	// Implement digigate rules here
	d.Transmitter.Transmit(msg)
	d.Logger.Info("Message retransmitted: ", msg)
}
