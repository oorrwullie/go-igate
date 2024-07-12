package main

import (
	"fmt"
	"io"
	"net/textproto"
	"strings"

	"github.com/pd0mz/go-aprs"
)

type AprsIs struct {
	id        string
	conn      *textproto.Conn
	connected bool
	cfg       Config
}

func NewAprsIs(cfg Config, logger *Logger) (*AprsIs, error) {
	if cfg.AprsIs.Options["server"] == "" {
		return nil, fmt.Errorf("no server specified")
	}

	if cfg.AprsIs.Options["call-sign"] == "" {
		return nil, fmt.Errorf("no callsign specified")
	}

	if cfg.AprsIs.Options["passcode"] == "" {
		return nil, fmt.Errorf("no passcode specified")
	}

	a := &AprsIs{
		id:        cfg.AprsIs.Options["call-sign"],
		conn:      nil,
		connected: false,
		cfg:       cfg,
	}

	err := a.Connect()
	if err != nil {
		return nil, err
	}

	logger.Debug(fmt.Sprintf("Connected to APRS-IS: %s", cfg.AprsIs.Options["server"]))

	go func() {
		for {
			msg, err := a.conn.ReadLine()
			if err != nil {
				logger.Error(err, "Error reading from APRS-IS")
				if err == io.EOF {
					logger.Info("Reconnecting to APRS-IS server")
					a.conn, err = textproto.Dial("tcp", cfg.AprsIs.Options["server"])
					if err != nil {
						logger.Error(err, "Could not reconnect to APRS-IS server.")
						a.conn.Close()
					}
					break
				} else if !isReadReceipt(msg) {
					p, err := a.ParsePacket(msg)
					if err != nil {
						logger.Error(err, "Could not parse packet from APRS-IS server.")
						continue
					}

					msg = fmt.Sprintf(
						"%s -> %s (%f, %f) %s",
						p.Src.Call,
						p.Dst.Call,
						p.Position.Latitude,
						p.Position.Longitude,
						p.Comment,
					)

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

	conn, err := textproto.Dial("tcp", a.cfg.AprsIs.Options["server"])
	if err != nil {
		return fmt.Errorf("could not connect to APRS-IS: %v", err)
	}

	err = conn.PrintfLine(
		"user %s pass %s vers Go-iGate 0.0.1 filter %s",
		a.cfg.AprsIs.Options["call-sign"],
		a.cfg.AprsIs.Options["passcode"],
		a.cfg.AprsIs.Options["filter"],
	)
	if err != nil {
		return err
	}

	resp, err := conn.ReadLine()
	if err != nil {
		return fmt.Errorf("could not read server response: %v", err)
	}

	if strings.HasPrefix(resp, fmt.Sprintf("# logresp %s verified", a.cfg.AprsIs.Options["call-sign"])) {
		return fmt.Errorf("APRS-IS server rejected connection: %s", resp)
	}

	a.conn = conn

	return nil
}

func (a *AprsIs) Disconnect() {
	if !a.connected {
		return
	}

	a.conn.Close()
	a.connected = false
}

func (a *AprsIs) Upload(p aprs.Packet) error {
	if !a.connected {
		err := a.Connect()
		if err != nil {
			return err
		}
	}

	err := a.conn.PrintfLine("%s", p.Raw)

	return err
}

func (a *AprsIs) ParsePacket(raw string) (aprs.Packet, error) {
	fmt.Printf("parsing packet: %s\n", raw)
	raw = strings.TrimPrefix(raw, "APRS: ")

	return aprs.ParsePacket(raw)
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
