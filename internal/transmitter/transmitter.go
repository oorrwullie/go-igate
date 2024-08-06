package transmitter

import (
	"fmt"
	"sync"
	"time"

	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/tarm/serial"
)

type Transmitter struct {
	Tx           *Tx
	stop         chan bool
	logger       *log.Logger
	serialConfig *serial.Config
}

type Tx struct {
	Chan chan string
	mu   sync.Mutex
}

func New(cfg config.Transmitter, logger *log.Logger) (*Transmitter, error) {
	dataPort, err := config.DetectDataPort()
	if err != nil {
		return nil, fmt.Errorf("Error detecting data port: %v", err)
	}

	timeout, err := time.ParseDuration(cfg.ReadTimeout)
	if err != nil {
		return nil, fmt.Errorf("Invalid read timeout: %v", err)
	}

	serialConfig := &serial.Config{
		Name:        dataPort,
		Baud:        cfg.BaudRate,
		ReadTimeout: timeout,
	}

	tx := &Tx{
		Chan: make(chan string, 1),
		mu:   sync.Mutex{},
	}

	return &Transmitter{
		Tx:           tx,
		stop:         make(chan bool),
		logger:       logger,
		serialConfig: serialConfig,
	}, nil
}

func (t *Transmitter) Start() error {
	go func() {
		for {
			select {
			case <-t.stop:
				return
			case msg := <-t.Tx.Chan:
				err := t.Transmit(msg)
				if err != nil {
					t.logger.Error("Error transmitting APRS message: ", err)
				}
			}
		}
	}()

	return nil
}

func (t *Transmitter) Stop() {
	t.stop <- true
}

func (t *Transmitter) Transmit(msg string) error {
	port, err := serial.OpenPort(t.serialConfig)
	if err != nil {
		return fmt.Errorf("failed to open serial port: %s", err)
	}

	fmtMsg := fmt.Sprintf("%v\r\n", msg)
	_, err = port.Write([]byte(fmtMsg))
	if err != nil {
		return fmt.Errorf("Error transmitting APRS message: %s", err)
	}

	t.logger.Info("APRS message transmitted: ", msg)

	port.Close()

	return nil
}

func (t *Tx) Send(msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Chan <- msg

	time.Sleep(time.Millisecond * 1500)
}

func (t *Tx) RxBackoff() {
	t.mu.Lock()
	defer t.mu.Unlock()

	time.Sleep(time.Millisecond * 1500)
}
