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
	Tx         *Tx
	stop       chan bool
	logger     *log.Logger
	serialMode *serial.Mode
	serialPort string
	timeout    time.Duration
}

type Tx struct {
	Chan chan string
	mu   sync.Mutex
}

func New(cfg config.Transmitter, logger *log.Logger) (*Transmitter, error) {
	sp, err := config.DetectDataPort()
	if err != nil {
		return nil, fmt.Errorf("Error detecting data port: %v", err)
	}

	timeout, err := time.ParseDuration(cfg.ReadTimeout)
	if err != nil {
		return nil, fmt.Errorf("Invalid read timeout: %v", err)
	}

	sm := &serial.Mode{
		BaudRate: cfg.BaudRate,
	}

	tx := &Tx{
		Chan: make(chan string, 1),
		mu:   sync.Mutex{},
	}

	return &Transmitter{
		Tx:         tx,
		stop:       make(chan bool),
		logger:     logger,
		serialMode: sm,
		serialPort: sp,
		timeout:    timeout,
	}, nil
}

func (t *Transmitter) Start() error {
	go func() {
		for {
			select {
			case <-t.stop:
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
	port, err := serial.Open(t.serialPort, t.serialMode)
	if err != nil {
		return fmt.Errorf("failed to open serial port: %s", err)
	}

	defer port.Close()

	port.SetReadTimeout(t.timeout)
	port.SetRTS(true)
	port.SetDTR(true)

	fmtMsg := fmt.Sprintf("%v\r\n", msg)
	writtenBytes, err := port.Write([]byte(fmtMsg))
	if err != nil {
		return fmt.Errorf("Error transmitting APRS message: %s", err)
	}

	fmt.Printf("APRS message transmitted: %s (wrote %d bytes)\n", msg, writtenBytes)

	time.Sleep(time.Second * 1)

	port.SetRTS(false)
	port.SetDTR(false)

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
