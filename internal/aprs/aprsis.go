package aprs

import (
	"fmt"
	"net/textproto"
	"strings"
	"sync/atomic"
	"time"

	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
)

type AprsIs struct {
	Callsign       string
	Inbound        chan string
	id             string
	Conn           *textproto.Conn
	connected      bool
	cfg            config.AprsIs
	logger         *log.Logger
	inboundEnabled atomic.Bool
	stop           chan struct{}
	stopOnce       atomic.Bool
}

func New(cfg config.AprsIs, callSign string, comment string, logger *log.Logger) (*AprsIs, error) {
	if cfg.Server == "" {
		return nil, fmt.Errorf("no server specified")
	}

	if callSign == "" {
		return nil, fmt.Errorf("no callsign specified")
	}

	if cfg.Passcode == "" {
		return nil, fmt.Errorf("no passcode specified")
	}

	a := &AprsIs{
		Callsign:  callSign,
		Inbound:   make(chan string, 64),
		Conn:      nil,
		connected: false,
		cfg:       cfg,
		stop:      make(chan struct{}),
	}

	err := a.Connect()
	if err != nil {
		return nil, err
	}

	logger.Debug(fmt.Sprintf("Connected to APRS-IS: %s", cfg.Server))

	go func() {
		for {
			msg, err := a.Conn.ReadLine()
			if err != nil {
				logger.Error(err, "Error reading from APRS-IS")
				select {
				case <-a.stop:
					return
				default:
				}

				logger.Info("Reconnecting to APRS-IS server")
				a.Disconnect()
				for {
					if err = a.Connect(); err == nil {
						break
					}
					logger.Error(err, "Could not reconnect to APRS-IS server.")
					select {
					case <-a.stop:
						return
					case <-time.After(5 * time.Second):
					}
				}
				continue
			}

			if !isReadReceipt(msg) && a.inboundEnabled.Load() {
				select {
				case a.Inbound <- msg:
				default:
					logger.Warn("Dropping APRS-IS packet: inbound queue full")
				}
				if !a.cfg.MuteInboundLog {
					logger.Info(fmt.Sprintf("%s %s", "[APRS-IS DIGIPEAT]", msg))
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

	conn, err := textproto.Dial("tcp", a.cfg.Server)
	if err != nil {
		return fmt.Errorf("could not connect to APRS-IS: %v", err)
	}

	err = conn.PrintfLine(
		"user %s pass %s vers DigiGate 0.0.1 filter %s",
		a.Callsign,
		a.cfg.Passcode,
		a.cfg.Filter,
	)
	if err != nil {
		conn.Close()
		return err
	}

	if err := verifyLoginResponse(conn, a.Callsign); err != nil {
		conn.Close()
		return err
	}

	fmt.Println("Connected to APRS-IS server")
	a.connected = true
	a.Conn = conn

	return nil
}

type lineReader interface {
	ReadLine() (string, error)
}

func verifyLoginResponse(reader lineReader, callSign string) error {
	for i := 0; i < 10; i++ {
		resp, err := reader.ReadLine()
		if err != nil {
			return fmt.Errorf("could not read server response: %v", err)
		}

		lowerResp := strings.ToLower(resp)
		if strings.HasPrefix(lowerResp, "# logresp ") {
			expected := fmt.Sprintf("# logresp %s verified", callSign)
			if !strings.HasPrefix(lowerResp, strings.ToLower(expected)) {
				return fmt.Errorf("APRS-IS server rejected connection: %s", resp)
			}
			return nil
		}
		if !strings.HasPrefix(resp, "#") {
			return fmt.Errorf("unexpected APRS-IS server response: %s", resp)
		}
	}
	return fmt.Errorf("APRS-IS server did not send a login response after greeting")
}

// SetInboundEnabled controls whether the reader retains APRS-IS packets for
// consumers. The connection may remain active for uploads even when message
// gating is disabled, so the reader must not fill an unused inbound queue.
func (a *AprsIs) SetInboundEnabled(enabled bool) {
	a.inboundEnabled.Store(enabled)
}

func (a *AprsIs) Disconnect() {
	if !a.connected {
		return
	}

	a.Conn.Close()
	a.connected = false
}

// Stop permanently ends the reader and closes the current connection.
func (a *AprsIs) Stop() {
	if a.stopOnce.CompareAndSwap(false, true) {
		close(a.stop)
	}
	a.Disconnect()
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
