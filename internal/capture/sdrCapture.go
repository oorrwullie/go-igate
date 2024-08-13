package capture

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"go.bug.st/serial"
)

type SdrCapture struct {
	cfg        config.Sdr
	logger     *log.Logger
	outputChan chan []byte
	Cmd        *exec.Cmd
	stop       chan bool
	port       serial.Port
}

func NewSdrCapture(cfg config.Sdr, outputChan chan []byte, logger *log.Logger) (*SdrCapture, error) {
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

	return &SdrCapture{
		cfg:        cfg,
		logger:     logger,
		outputChan: outputChan,
		Cmd:        cmd,
		stop:       make(chan bool),
		port:       nil,
	}, nil
}

func (s *SdrCapture) Start() error {
	out, err := s.Cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("Error reading rtl_fm stdout: %s", err.Error())
	}

	err = s.Cmd.Start()
	if err != nil {
		return fmt.Errorf("Error starting rtl_fm: %v", err)
	}

	go func() {
		select {
		case <-s.stop:
			s.logger.Info("Stopping rtl_fm process")
			s.Cmd.Process.Kill()
		default:
			buf := make([]byte, 4096)

			s.pipeRtlFM(out, buf)
		}
	}()

	return nil
}

func (s *SdrCapture) Stop() {
	s.stop <- true
}

func (s *SdrCapture) Port() serial.Port {
	return s.port
}

// Read from the rtl_fm stdout and send to the output channel
func (s *SdrCapture) pipeRtlFM(out io.ReadCloser, buf []byte) {
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

			s.outputChan <- data
		}
	}
}
