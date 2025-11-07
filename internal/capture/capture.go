package capture

import (
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
)

type Capture interface {
	Start() error
	Stop()
	Type() string
}

func New(cfg config.Config, outputChan chan []byte, logger *log.Logger) (Capture, error) {
    if cfg.SoundcardCapture {
        return NewSoundcardCapture(cfg, outputChan, logger)
    }

    if cfg.Sdr.Enabled {
        return NewSdrCapture(cfg.Sdr, outputChan, logger)
    }

    return NewSoundcardCapture(cfg, outputChan, logger)
}
