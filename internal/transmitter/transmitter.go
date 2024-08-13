package transmitter

import (
	"fmt"
	"sync"
	"time"

	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"go.bug.st/serial"
)

type Transmitter struct {
	Tx     *Tx
	stop   chan bool
	logger *log.Logger
	Port   serial.Port
}

type Tx struct {
	Chan chan string
	mu   sync.Mutex
}

func New(cfg config.Transmitter, port serial.Port, logger *log.Logger) (*Transmitter, error) {
	sp, err := config.DetectDataPort()
	if err != nil {
		return nil, fmt.Errorf("Error detecting data port: %v", err)
	}

	sm := &serial.Mode{
		BaudRate: cfg.BaudRate,
	}

	tx := &Tx{
		Chan: make(chan string, 1),
		mu:   sync.Mutex{},
	}

	if port == nil {
		port, err = serial.Open(sp, sm)
		if err != nil {
			return nil, fmt.Errorf("failed to open serial port: %s", err)
		}

		timeout, err := time.ParseDuration(cfg.ReadTimeout)
		if err != nil {
			return nil, fmt.Errorf("Invalid read timeout: %v", err)
		}

		port.SetReadTimeout(timeout)
	}

	return &Transmitter{
		Tx:     tx,
		stop:   make(chan bool),
		logger: logger,
		Port:   port,
	}, nil
}

func (t *Transmitter) Start() error {
	go func() {
		for {
			select {
			case <-t.stop:
				t.Port.Close()
				return
			case msg := <-t.Tx.Chan:
				fmt.Printf("tx received message: %v\n", msg)
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
	t.Port.SetRTS(true)
	t.Port.SetDTR(true)

	fmtMsg := fmt.Sprintf("%v\r\n", msg)
	writtenBytes, err := t.Port.Write([]byte(fmtMsg))
	if err != nil {
		return fmt.Errorf("Error transmitting APRS message: %s", err)
	}

	fmt.Printf("APRS message transmitted: %s (wrote %d bytes)\n", msg, writtenBytes)

	time.Sleep(time.Second * 1)

	t.Port.SetRTS(false)
	t.Port.SetDTR(false)

	time.Sleep(time.Second * 2)
	t.logger.Info("APRS message transmitted: ", msg)

	return nil
}

func (t *Tx) Send(msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Chan <- msg
	fmt.Printf("Tx sent message to tx: %v\n", msg)

	time.Sleep(time.Millisecond * 1500)
}

func (t *Tx) RxBackoff() {
	t.mu.Lock()
	defer t.mu.Unlock()

	time.Sleep(time.Millisecond * 3500)
	fmt.Printf("tx backoff finished")
}
