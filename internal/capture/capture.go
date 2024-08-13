package capture

import (
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"go.bug.st/serial"
)

type Capture interface {
	Start() error
	Stop()
	Port() serial.Port
}

func New(cfg config.Config, outputChan chan []byte, logger *log.Logger) (Capture, error) {
	if cfg.Sdr.Enabled {
		return NewSdrCapture(cfg.Sdr, outputChan, logger)
	}

	return NewSerialCapture(cfg.Transmitter, outputChan, logger)
}
