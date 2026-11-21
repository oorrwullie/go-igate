package capture

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/oorrwullie/sparky/internal/config"
	"github.com/oorrwullie/sparky/internal/log"
)

type SdrCapture struct {
	cfg        config.Sdr
	logger     *log.Logger
	outputChan chan []byte
	Cmd        *exec.Cmd
	stop       chan struct{}
	stopOnce   sync.Once
	done       chan struct{}
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
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
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
		defer close(s.done)
		buf := make([]byte, 4096)
		s.pipeRtlFM(out, buf)
		_ = s.Cmd.Wait()
	}()

	return nil
}

func (s *SdrCapture) Stop() {
	s.stopOnce.Do(func() {
		close(s.stop)
		if s.Cmd.Process != nil {
			_ = s.Cmd.Process.Kill() // Interrupt a blocked stdout read.
		}
		if s.done != nil {
			<-s.done // Ensure the process was reaped before shutdown completes.
		}
	})
}

func (s *SdrCapture) Type() string {
	return "Sdr"
}

// Read from the rtl_fm stdout and send to the output channel
func (s *SdrCapture) pipeRtlFM(out io.ReadCloser, buf []byte) {
	for {
		n, err := out.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			select {
			case <-s.stop:
				return
			default:
				s.logger.Error("Error reading rtl_fm stdout:", err)
				return
			}
		}

		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])

			select {
			case <-s.stop:
				return
			case s.outputChan <- data:
			}
		}
	}
}
