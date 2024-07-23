package aprs

import (
	"fmt"
	"io"
	"net/textproto"
	"strings"

	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
)

type AprsIs struct {
	id        string
	Conn      *textproto.Conn
	connected bool
	cfg       config.AprsIs
	logger    *log.Logger
}

func NewAprsIs(cfg config.AprsIs, logger *log.Logger) (*AprsIs, error) {
	if cfg.Options["server"] == "" {
		return nil, fmt.Errorf("no server specified")
	}

	if cfg.Options["call-sign"] == "" {
		return nil, fmt.Errorf("no callsign specified")
	}

	if cfg.Options["passcode"] == "" {
		return nil, fmt.Errorf("no passcode specified")
	}

	a := &AprsIs{
		id:        cfg.Options["call-sign"],
		Conn:      nil,
		connected: false,
		cfg:       cfg,
	}

	err := a.Connect()
	if err != nil {
		return nil, err
	}

	logger.Debug(fmt.Sprintf("Connected to APRS-IS: %s", cfg.Options["server"]))

	go func() {
		for {
			msg, err := a.Conn.ReadLine()
			if err != nil {
				logger.Error(err, "Error reading from APRS-IS")
				if err == io.EOF {
					logger.Info("Reconnecting to APRS-IS server")
					a.Disconnect()
					err = a.Connect()
					if err != nil {
						logger.Error(err, "Could not reconnect to APRS-IS server.")
						a.Disconnect()
					}
					break
				} else if !isReadReceipt(msg) {
					logger.Info(
						fmt.Sprintf(
							"%s %s",
							"[APRS-IS DIGIPEAT]",
							msg,
						),
					)

				}
			}
		}
	}()

	return a, nil
}

func (a *AprsIs) Connect() error {
	if a.connected {
		return nil
	}

	conn, err := textproto.Dial("tcp", a.cfg.Options["server"])
	if err != nil {
		return fmt.Errorf("could not connect to APRS-IS: %v", err)
	}

	err = conn.PrintfLine(
		"user %s pass %s vers Go-iGate 0.0.1 filter %s",
		a.cfg.Options["call-sign"],
		a.cfg.Options["passcode"],
		a.cfg.Options["filter"],
	)
	if err != nil {
		return err
	}

	resp, err := conn.ReadLine()
	if err != nil {
		return fmt.Errorf("could not read server response: %v", err)
	}

	if strings.HasPrefix(resp, fmt.Sprintf("# logresp %s verified", a.cfg.Options["call-sign"])) {
		return fmt.Errorf("APRS-IS server rejected connection: %s", resp)
	}

	a.Conn = conn

	return nil
}

func (a *AprsIs) Disconnect() {
	if !a.connected {
		return
	}

	a.Conn.Close()
	a.connected = false
}

func (a *AprsIs) Upload(msg string) error {
	if !a.connected {
		err := a.Connect()
		if err != nil {
			return err
		}
	}

	msg = strings.TrimPrefix(msg, "APRS: ")

	err := a.Conn.PrintfLine("%s", msg)

	return err
}

func isReadReceipt(message string) bool {
	if strings.HasPrefix(message, "# aprsc") {
		return true
	}

	if strings.HasPrefix(message, "# javAPRSSrvr") {
		return true
	}

	return false
}
