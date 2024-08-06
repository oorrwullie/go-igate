package sdr

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/transmitter"
)

type Sdr struct {
	cfg        config.Sdr
	logger     *log.Logger
	outputChan chan []byte
	Cmd        *exec.Cmd
	tx         *transmitter.Tx
}

func New(cfg config.Sdr, outputChan chan []byte, tx *transmitter.Tx, logger *log.Logger) *Sdr {
	requiredArgs := []string{
		"-f",
		cfg.Frequency,
		"-s",
		cfg.SampleRate,
		"-l",
		cfg.SquelchLevel,
		"-g",
		cfg.Gain,
		"-p",
		cfg.PpmError,
	}

	if cfg.Device != "" && cfg.Device != "0" {
		logger.Info("Using device ", cfg.Device)
		requiredArgs = append(requiredArgs, "-d", cfg.Device)
	}

	userArgs := strings.Fields(cfg.AdditionalFlags)
	args := append(requiredArgs, userArgs...)
	args = append(args, "-")
	cmd := exec.Command("rtl_fm", args...)

	return &Sdr{
		cfg:        cfg,
		logger:     logger,
		outputChan: outputChan,
		Cmd:        cmd,
		tx:         tx,
	}
}

func (s *Sdr) Start() error {
	out, err := s.Cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("Error reading rtl_fm stdout: %s", err.Error())
	}

	err = s.Cmd.Start()
	if err != nil {
		return fmt.Errorf("Error starting rtl_fm: %v", err)
	}

	go func() {
		buf := make([]byte, 4096)

		s.pipeRtlFM(out, buf)
	}()

	return nil
}

// Read from the rtl_fm stdout and send to the output channel
func (s *Sdr) pipeRtlFM(out io.ReadCloser, buf []byte) {
	for {
		n, err := out.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			s.logger.Error("Error reading rtl_fm stdout:", err)
			return
		}

		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])

			if s.tx != nil {
				go s.tx.RxBackoff()
			}

			s.outputChan <- data
		}
	}
}
