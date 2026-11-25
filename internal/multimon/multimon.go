package multimon

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oorrwullie/sparky/internal/cache"
	"github.com/oorrwullie/sparky/internal/config"
	"github.com/oorrwullie/sparky/internal/log"
	"github.com/oorrwullie/sparky/internal/observability"
	"github.com/oorrwullie/sparky/internal/pubsub"
	"github.com/oorrwullie/sparky/internal/transmitter"
)

type Multimon struct {
	cache           *cache.Cache
	cfg             config.Multimon
	logger          *log.Logger
	inputChan       chan []byte
	pubsub          *pubsub.PubSub
	Cmd             *exec.Cmd
	tx              *transmitter.Tx
	stationCallsign string
	inputBytes      atomic.Uint64
	Reception observability.Reception
	decoderRunning atomic.Bool
	stopHealth      chan struct{}
	stopHealthOnce  sync.Once
	lifecycleMu     sync.Mutex
	stopCh          chan struct{}
	done            chan struct{}
	started         bool
	stopped         bool
}

// New creates a new multimon-ng instance
func New(cfg config.Multimon, inputChan chan []byte, ps *pubsub.PubSub, cache *cache.Cache, tx *transmitter.Tx, stationCallsign string, logger *log.Logger) *Multimon {
	requiredArgs := []string{
		"-a",
		"AFSK1200",
		"-A",
		"-t",
		"raw",
	}

	userArgs := strings.Fields(cfg.AdditionalFlags)
	args := append(requiredArgs, userArgs...)
	args = append(args, "-")

	binary := cfg.Path
	if binary == "" {
		binary = "multimon-ng"
	}

	cmd := exec.Command(binary, args...)

	return &Multimon{
		cache:           cache,
		cfg:             cfg,
		logger:          logger,
		inputChan:       inputChan,
		pubsub:          ps,
		Cmd:             cmd,
		tx:              tx,
		stationCallsign: strings.ToUpper(strings.TrimSpace(stationCallsign)),
		stopHealth:      make(chan struct{}),
		stopCh:          make(chan struct{}),
	}
}

// Start owns exactly one child process and its corresponding Cmd.Wait.
// Startup completes synchronously so callers cannot race an unstarted child.
func (m *Multimon) Start() error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.stopped {
		return fmt.Errorf("multimon-ng already stopped")
	}
	if m.started {
		return nil
	}
	if m.stopCh == nil {
		m.stopCh = make(chan struct{})
	}
	inPipe, err := m.Cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("multimon-ng stdin: %w", err)
	}
	outPipe, err := m.Cmd.StdoutPipe()
	if err != nil {
		_ = inPipe.Close()
		return fmt.Errorf("multimon-ng stdout: %w", err)
	}
	if err := m.Cmd.Start(); err != nil {
		_ = inPipe.Close()
		_ = outPipe.Close()
		return fmt.Errorf("start multimon-ng: %w", err)
	}
	m.started = true
	m.done = make(chan struct{})
	m.logger.Info("multimon-ng decoder started: ", m.Cmd.Path, " ", strings.Join(m.Cmd.Args[1:], " "))
	if m.cfg.ReceiveDiagnostics {
		go m.logInputHealth()
	}
	m.decoderRunning.Store(true)
	readerDone := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		defer inPipe.Close()
		for {
			select {
			case <-m.stopCh:
				return
			case <-readerDone:
				return
			case data, ok := <-m.inputChan:
				if !ok {
					return
				}
				n, err := inPipe.Write(data)
				if n > 0 && m.cfg.ReceiveDiagnostics {
					m.inputBytes.Add(uint64(n))
				}
				if err != nil {
					m.logger.Error("Error writing to multimon-ng: ", err)
					return
				}
			}
		}
	}()
	go func() {
		defer close(readerDone)
		defer m.decoderRunning.Store(false)
		defer m.stopInputHealth()
		scanner := bufio.NewScanner(outPipe)
		for scanner.Scan() {
			msg := scanner.Text()
			normalized := normalizeMultimonMessage(msg)
			if normalized == "" {
				continue
			}
			m.logger.Debug("Got message: ", msg)
			now := time.Now()
			m.Reception.Received(now)
			duplicate := m.cache.Set(normalized, now)
			if duplicate && !m.isSelfMessage(normalized) {
				m.Reception.Suppressed()
				m.logger.Info("Duplicate packet received: ", msg)
				continue
			}
			if duplicate {
				m.logger.Debug("Allowing duplicate for station beacon: ", normalized)
			} else {
				m.logger.Info("packet received: ", msg)
			}
			m.pubsub.Publish(normalized)
		}
		if err := scanner.Err(); err != nil {
			m.logger.Error("Error reading from multimon-ng: ", err)
		}
	}()
	go func() {
		<-readerDone
		<-writerDone
		err := m.Cmd.Wait()
		if err != nil {
			select {
			case <-m.stopCh:
			default:
				m.logger.Warn("multimon-ng exited: ", err)
			}
		}
		close(m.done)
	}()
	return nil
}

// Stop terminates the decoder and waits until the sole Cmd.Wait reaps it.
// Never-started and failed-start instances return without blocking.
func (m *Multimon) Stop() {
	m.lifecycleMu.Lock()
	if !m.stopped {
		m.stopped = true
		if m.stopCh == nil {
			m.stopCh = make(chan struct{})
		}
		close(m.stopCh)
		m.stopInputHealth()
	}
	done := m.done
	if m.started && m.Cmd.Process != nil {
		_ = m.Cmd.Process.Kill()
	}
	m.lifecycleMu.Unlock()
	if done != nil {
		<-done
	}
}

func (m *Multimon) logInputHealth() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopHealth:
			return
		case <-ticker.C:
			m.logger.Info("multimon-ng input diagnostic: audio-bytes=", m.inputBytes.Swap(0), " in the last 15s")
		}
	}
}

func (m *Multimon) stopInputHealth() {
	m.stopHealthOnce.Do(func() { close(m.stopHealth) })
}

func normalizeMultimonMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	msg = strings.TrimPrefix(msg, "APRS: ")
	if !strings.Contains(msg, ">") {
		return ""
	}
	return msg
}

func (m *Multimon) isSelfMessage(msg string) bool {
	if m.stationCallsign == "" {
		return false
	}

	frame := strings.TrimSpace(msg)
	if frame == "" {
		return false
	}

	parts := strings.SplitN(frame, ">", 2)
	if len(parts) == 0 {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(parts[0]), m.stationCallsign)
}

func (m *Multimon) DecoderRunning() bool { return m.decoderRunning.Load() }
