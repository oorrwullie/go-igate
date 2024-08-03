package multimon

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/oorrwullie/go-igate/internal/cache"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
)

type Multimon struct {
	cache      *cache.Cache
	cfg        config.Multimon
	logger     *log.Logger
	inputChan  chan []byte
	outputChan chan string
	Cmd        *exec.Cmd
}

// New creates a new multimon-ng instance
func New(cfg config.Multimon, inputChan chan []byte, outputChan chan string, cache *cache.Cache, logger *log.Logger) *Multimon {
	requiredArgs := []string{
		"-a",
		"AFSK1200",
		"-A",
		"-t",
		"raw",
	}

	userArgs := strings.Fields(cfg.AdditionalFlags)
	args := append(requiredArgs, userArgs...)
	args = append(args, "-")
	cmd := exec.Command("multimon-ng", args...)

	return &Multimon{
		cache:      cache,
		cfg:        cfg,
		logger:     logger,
		inputChan:  inputChan,
		outputChan: outputChan,
		Cmd:        cmd,
	}
}

// Start starts the multimon-ng process and reads from the input and writes to the output channel
func (m *Multimon) Start() error {
	// Start the multimon-ng process
	go func() {
		inPipe, err := m.Cmd.StdinPipe()
		if err != nil {
			m.logger.Error("Error opening multimon-ng stdin:", err)
			return
		}

		outPipe, err := m.Cmd.StdoutPipe()
		if err != nil {
			m.logger.Error("Error opening multimon-ng stdout: ", err)
			return
		}

		err = m.Cmd.Start()
		if err != nil {
			m.logger.Error("Error starting multimon-ng: ", err)
			return
		}

		// Start goroutines to read from the input channel and write to the output channel
		go func(in io.WriteCloser) {
			defer in.Close()

			for data := range m.inputChan {
				fmt.Println("multimon-ng input channel message received")
				_, err := in.Write(data)
				if err != nil {
					m.logger.Error("Error writing to multimon-ng: ", err)
				}
			}
		}(inPipe)

		// This is reading from the output of multimon-ng and writing to the output channel
		// This is where the APRS packets are read from multimon-ng
		// and sent to the output channel
		go func(out io.ReadCloser) {
			scanner := bufio.NewScanner(out)
			for scanner.Scan() {
				msg := scanner.Text()
				if exists := m.cache.Set(msg, time.Now()); !exists {
					m.logger.Info("packet received: ", msg)

					m.outputChan <- msg
					fmt.Printf("message sent to multimon output channel: %v/n", msg)
				} else {
					m.logger.Info("Duplicate packet received: ", msg)
				}
			}

			if err := scanner.Err(); err != nil {
				m.logger.Error("Error reading from multimon-ng: ", err)
			}

		}(outPipe)
	}()

	return nil
}
