package transmitter

import (
	"fmt"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/tarm/serial"
	"time"
)

type Transmitter struct {
	TxChan chan string
	Stop   chan bool
	Logger *log.Logger
}

func (t *Transmitter) StartTx() error {
	dataPort, err := config.DetectDataPort()
	if err != nil {
		return fmt.Errorf("Error detecting data port: %v", err)
	}

	c := &serial.Config{
		Name:        dataPort,
		Baud:        1200,
		ReadTimeout: time.Second * 5,
	}

	go func() {
		for {
			select {
			case <-t.Stop:
				return
			case msg := <-t.TxChan:
				port, err := serial.OpenPort(c)
				if err != nil {
					t.Logger.Error("failed to open serial port: ", err)
				}

				fmtMsg := fmt.Sprintf("%v\r\n", msg)
				_, err = port.Write([]byte(fmtMsg))
				if err != nil {
					t.Logger.Error("failed to write to serial port: ", err)
				}

				t.Logger.Info("APRS message transmitted: ", msg)

				if err != nil {
					t.Logger.Error("Error transmitting APRS message: ", err)
				}

				port.Close()
			}
		}
	}()

	return nil
}

func (t *Transmitter) StopTx() {
	t.Stop <- true
}

func (t *Transmitter) Transmit(msg string) {
	t.TxChan <- msg
}
