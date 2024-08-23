package transmitter

import (
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/oorrwullie/go-igate/internal/capture"
	"github.com/oorrwullie/go-igate/internal/log"
)

type Transmitter struct {
	Tx        *Tx
	stop      chan bool
	logger    *log.Logger
	soundcard *capture.SoundcardCapture
}

type Tx struct {
	Chan chan string
	mu   sync.Mutex
}

type waveReader struct {
	wave     []byte
	position int
}

func New(sc *capture.SoundcardCapture, logger *log.Logger) (*Transmitter, error) {
	tx := &Tx{
		Chan: make(chan string, 1),
		mu:   sync.Mutex{},
	}

	return &Transmitter{
		Tx:        tx,
		stop:      make(chan bool),
		logger:    logger,
		soundcard: sc,
	}, nil
}

func (t *Transmitter) Start() error {
	go func() {
		for {
			select {
			case <-t.stop:
				return
			case msg := <-t.Tx.Chan:
				t.Transmit(msg)
			}
		}
	}()

	return nil
}

func (t *Transmitter) Stop() {
	t.logger.Info("Stopping soundcard capture...")
	t.soundcard.Stop()

	t.logger.Info("Stopping transmitter...")
	t.stop <- true
}

func (t *Transmitter) Transmit(msg string) {
	fmtMsg := fmt.Sprintf("%v\r\n", msg)
	t.soundcard.Play(fmtMsg)
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
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Chan <- msg
	// time.Sleep(time.Millisecond * 1500)
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
