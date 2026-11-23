package aprs

import (
	"context"
	"fmt"
	"net"
	"sync"
	"net/textproto"
	"strings"
	"sync/atomic"
	"time"

	"github.com/oorrwullie/sparky/internal/config"
	"github.com/oorrwullie/sparky/internal/log"
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
	connMu         sync.Mutex
	connectMu      sync.Mutex
	writeMu        sync.Mutex
	pending        net.Conn
	ctx            context.Context
	cancel         context.CancelFunc
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

	ctx, cancel := context.WithCancel(context.Background())
	a := &AprsIs{
		ctx:       ctx,
		cancel:    cancel,
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
			conn := a.currentConnection()
			if conn == nil {
				select {
				case <-a.stop:
					return
				default:
				}
				if err := a.Connect(); err != nil {
					logger.Error(err, "Could not connect to APRS-IS server")
					select {
					case <-a.stop:
						return
					case <-time.After(5 * time.Second):
						continue
					}
				}
				continue
			}
			msg, err := conn.ReadLine()
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

func (a *AprsIs) isStopped() bool {
	return a.stopOnce.Load()
}

func (a *AprsIs) currentConnection() *textproto.Conn {
	a.connMu.Lock()
	defer a.connMu.Unlock()
	return a.Conn
}

// Connect serializes connection attempts but never holds connMu across I/O.
// Stop can therefore close a stalled login socket immediately.
func (a *AprsIs) Connect() error {
	a.connectMu.Lock()
	defer a.connectMu.Unlock()
	if a.isStopped() {
		return fmt.Errorf("APRS-IS client stopped")
	}
	if a.currentConnection() != nil {
		return nil
	}

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	raw, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", a.cfg.Server)
	if err != nil {
		return fmt.Errorf("could not connect to APRS-IS: %w", err)
	}
	a.connMu.Lock()
	if a.isStopped() {
		a.connMu.Unlock()
		raw.Close()
		return fmt.Errorf("APRS-IS client stopped")
	}
	a.pending = raw
	a.connMu.Unlock()
	defer func() {
		a.connMu.Lock()
		if a.pending == raw {
			a.pending = nil
		}
		a.connMu.Unlock()
	}()

	// A server that accepts TCP but never sends logresp must not pin shutdown.
	if err := raw.SetDeadline(time.Now().Add(10*time.Second)); err != nil {
		raw.Close()
		return err
	}
	conn := textproto.NewConn(raw)
	if err := conn.PrintfLine("user %s pass %s vers Sparky 0.0.1 filter %s",
		a.Callsign, a.cfg.Passcode, a.cfg.Filter); err != nil {
		conn.Close()
		return err
	}
	if err := verifyLoginResponse(conn, a.Callsign); err != nil {
		conn.Close()
		return err
	}
	if err := raw.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return err
	}
	a.connMu.Lock()
	if a.isStopped() {
		a.connMu.Unlock()
		conn.Close()
		return fmt.Errorf("APRS-IS client stopped")
	}
	a.pending = nil
	a.Conn = conn
	a.connected = true
	a.connMu.Unlock()
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
	a.connMu.Lock()
	conn := a.Conn
	a.Conn = nil
	a.connected = false
	a.connMu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// Stop closes both the active and not-yet-logged-in connections; it never
// waits behind Connect's network I/O. Further reconnects are rejected.
func (a *AprsIs) Stop() {
	if a.stopOnce.CompareAndSwap(false, true) {
		if a.cancel != nil {
			a.cancel()
		}
		if a.stop != nil {
			close(a.stop)
		}
	}
	a.connMu.Lock()
	pending := a.pending
	active := a.Conn
	a.pending = nil
	a.Conn = nil
	a.connected = false
	a.connMu.Unlock()
	if pending != nil {
		_ = pending.Close()
	}
	if active != nil {
		_ = active.Close()
	}
}

// Upload serializes all APRS-IS writes, including concurrent beacons and RF
// packet uploads. Stop never takes writeMu, so it can interrupt blocked I/O.
func (a *AprsIs) Upload(msg string) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	if a.isStopped() {
		return fmt.Errorf("APRS-IS client stopped")
	}
	conn := a.currentConnection()
	if conn == nil {
		if err := a.Connect(); err != nil {
			return err
		}
		conn = a.currentConnection()
		if conn == nil {
			return fmt.Errorf("APRS-IS connection unavailable")
		}
	}
	msg = strings.TrimPrefix(msg, "APRS: ")
	return conn.PrintfLine("%s", msg)
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
