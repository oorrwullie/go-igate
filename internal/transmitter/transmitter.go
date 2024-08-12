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
	sp, err := config.DetectDataPort()
	if err != nil {
		return nil, fmt.Errorf("Error detecting data port: %v", err)
	}

	sc := &serial.Config{
		Baud: cfg.BaudRate,
		Name: sp,
	}

	tx := &Tx{
		Chan: make(chan string, 1),
		mu:   sync.Mutex{},
	}

	return &Transmitter{
		Tx:           tx,
		stop:         make(chan bool),
		logger:       logger,
		serialConfig: sc,
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
	port, err := serial.OpenPort(t.serialConfig)
	if err != nil {
		return fmt.Errorf("failed to open serial port: %s", err)
	}

	defer port.Close()

	fmtMsg := fmt.Sprintf("%v\r\n", msg)
	_, err = port.Write([]byte(fmtMsg))
	if err != nil {
		return fmt.Errorf("Error transmitting APRS message: %s", err)
	}

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
