package capture

import (
	"fmt"
	"time"

	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"

	"go.bug.st/serial"
)

type SerialCapture struct {
	cfg        config.Transmitter
	logger     *log.Logger
	outputChan chan []byte
	serialMode *serial.Mode
	serialPort string
	timeout    time.Duration
	stop       chan bool
	port       serial.Port
}

func NewSerialCapture(cfg config.Transmitter, outputChan chan []byte, logger *log.Logger) (*SerialCapture, error) {
	sp, err := config.DetectDataPort()
	if err != nil {
		return nil, fmt.Errorf("Error detecting data port: %v", err)
	}

	sm := &serial.Mode{
		BaudRate: cfg.BaudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(sp, sm)
	if err != nil {
		return nil, fmt.Errorf("failed to open serial port: %s", err)
	}

	timeout, err := time.ParseDuration(cfg.ReadTimeout)
	if err != nil {
		return nil, fmt.Errorf("Invalid read timeout: %v", err)
	}

	port.SetMode(sm)
	port.SetReadTimeout(timeout)

	return &SerialCapture{
		cfg:        cfg,
		logger:     logger,
		outputChan: outputChan,
		serialMode: sm,
		serialPort: sp,
		timeout:    timeout,
		stop:       make(chan bool),
		port:       port,
	}, nil
}

func (sc *SerialCapture) Start() error {
	var (
		buf     = make([]byte, 256)
		readbuf = make([]byte, 64)
	)

	go func() {
		for {
			select {
			case <-sc.stop:
				sc.port.Close()
				return
			default:
				n, err := sc.port.Read(readbuf)
				if err != nil {
					sc.logger.Error("Error reading serial port:", err)
					continue
				}

				if n > 0 {
					buf = append(buf, readbuf[:n]...)

					for {
						nli := sc.findNewline(buf)

						if nli == -1 {
							break
						}

						data := make([]byte, nli)
						copy(data, buf[:nli])
						dbg := fmt.Sprintf("Received %d bytes: % X", n, buf[:n])
						sc.logger.Debug(dbg)

						sc.outputChan <- data
						buf = buf[nli+1:]
					}
				}
			}
		}

	}()

	return nil
}

func (sc *SerialCapture) Stop() {
	sc.stop <- true
}

func (sc *SerialCapture) Port() serial.Port {
	return sc.port
}

func (sc *SerialCapture) findNewline(buffer []byte) int {
	for i := 0; i < len(buffer)-1; i++ {
		if buffer[i] == '\r' && buffer[i+1] == '\n' {
			return i
		}
	}
	for i := 0; i < len(buffer); i++ {
		if buffer[i] == '\n' || buffer[i] == '\r' {
			return i
		}
	}
	return -1
}
