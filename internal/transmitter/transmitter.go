package transmitter

import (
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/oorrwullie/sparky/internal/capture"
	"github.com/oorrwullie/sparky/internal/log"
)

// Backend handles hardware-specific transmission and shutdown. RF scheduling
// remains in the iGate/digipeater layers; only their admitted frames reach here.
//
// Shutdown contract: Stop is terminal, concurrency-safe and idempotent. It
// must interrupt Play already in progress and ensure Play invoked concurrently
// with or after Stop returns promptly without starting new hardware activity.
// The transmitter cannot serialize Stop behind Play: Play may itself block
// waiting for hardware, and Stop is what must unblock it.
// Play may return once a frame is queued for hardware; it need not wait for
// transmission to finish.
type Backend interface {
	Play(string)
	Stop()
}

type Transmitter struct {
	Tx        *Tx
	stop       chan struct{}
	stopOnce   sync.Once
	mu         sync.Mutex
	started    bool
	workerDone chan struct{}
	logger     *log.Logger
	backend    Backend
	soundcard  *capture.SoundcardCapture // Legacy AFSK helpers; not the dispatch path.
}

type Tx struct {
	Chan chan string
	mu   sync.Mutex
	stop <-chan struct{}
}

type waveReader struct {
	wave     []byte
	position int
}

// New retains the existing sound-card wiring for callers.
func New(sc *capture.SoundcardCapture, logger *log.Logger) (*Transmitter, error) {
	if sc == nil {
		return nil, fmt.Errorf("sound-card transmitter backend is nil")
	}
	t, err := NewWithBackend(sc, logger)
	if err != nil {
		return nil, err
	}
	t.soundcard = sc
	return t, nil
}

// NewWithBackend permits a new transmitter implementation without changing
// queue admission, quiet-channel gating, or packet scheduling.
func NewWithBackend(backend Backend, logger *log.Logger) (*Transmitter, error) {
	if backend == nil {
		return nil, fmt.Errorf("transmitter backend is nil")
	}
	stop := make(chan struct{})
	return &Transmitter{
		Tx:      &Tx{Chan: make(chan string, 1), stop: stop},
		stop:    stop,
		logger:  logger,
		backend: backend,
	}, nil
}

func (t *Transmitter) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	select {
	case <-t.stop:
		return fmt.Errorf("transmitter already stopped")
	default:
	}
	if t.started {
		return nil
	}
	t.started = true
	t.workerDone = make(chan struct{})
	go func() {
		defer close(t.workerDone)
		for {
			select {
			case <-t.stop:
				return
			case msg := <-t.Tx.Chan:
				// A selected queued frame must not be transmitted after stop.
				select {
				case <-t.stop:
					return
				default:
				}
				t.Transmit(msg)
			}
		}
	}()
	return nil
}

// Stop first cancels packet admission, then releases any hardware-blocked
// Play call, and finally waits for the dedicated transmit worker to exit.
func (t *Transmitter) Stop() {
	t.stopOnce.Do(func() {
		t.mu.Lock()
		// Publish and shutdown share the queue mutex. Once Stop returns,
		// no sender can have admitted a frame after cancellation.
		t.Tx.mu.Lock()
		close(t.stop)
		t.Tx.mu.Unlock()
		done := t.workerDone
		t.mu.Unlock()
		t.backend.Stop()
		if done != nil {
			<-done
		}
		if t.logger != nil {
			t.logger.Info("Transmitter stopped")
		}
	})
}

func (t *Transmitter) Transmit(msg string) {
	t.backend.Play(msg)
}

func (t *Transmitter) encodeMsg(msg string) []byte {
	var (
		bitDuration = time.Second / time.Duration(t.soundcard.BaudRate)
		signal      []byte
	)

	for _, bit := range msg {
		var freq float64
		if bit == '1' {
			freq = float64(t.soundcard.MarkFrequency)
		} else {
			freq = float64(t.soundcard.SpaceFrequency)
		}
		signal = append(signal, t.generateSineWave(freq, bitDuration, 0.5)...)
	}

	return signal
}

func (t *Transmitter) generateSineWave(frequency float64, duration time.Duration, amplitude float64) []byte {
	samples := int(float64(t.soundcard.SampleRate) * duration.Seconds())
	wave := make([]byte, samples*t.soundcard.BitDepth)

	for i := 0; i < samples; i++ {
		t := float64(i) / float64(t.soundcard.SampleRate)
		value := int16(amplitude * math.Sin(2*math.Pi*frequency*t) * 32767)
		wave[i*2] = byte(value)
		wave[i*2+1] = byte(value >> 8)
	}

	return wave
}

func (t *Tx) Send(msg string) {
	t.SendUntil(msg, nil)
}

// SendUntil queues a frame unless stop is closed first. A nil stop channel
// retains the historical blocking behavior used by simple callers.
func (t *Tx) SendUntil(msg string, stop <-chan struct{}) bool {
	for {
		select {
		case <-t.stop:
			return false
		case <-stop:
			return false
		default:
		}
		if t.TrySend(msg) {
			return true
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-t.stop:
			timer.Stop()
			return false
		case <-stop:
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}

// TrySend queues msg only when a channel slot is immediately available.
func (t *Tx) TrySend(msg string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	select {
	case <-t.stop:
		return false
	default:
	}
	select {
	case t.Chan <- msg:
		return true
	default:
		return false
	}
}

func (t *Tx) RxBackoff() {
	t.mu.Lock()
	defer t.mu.Unlock()

	time.Sleep(time.Millisecond * 3500)
	fmt.Printf("tx backoff finished")
}

func NewWaveReader(wave []byte) *waveReader {
	return &waveReader{
		wave: wave,
	}
}

func (w *waveReader) Read(p []byte) (int, error) {
	if w.position >= len(w.wave) {
		return 0, io.EOF
	}

	n := copy(p, w.wave[w.position:])
	w.position += n

	return n, nil
}
