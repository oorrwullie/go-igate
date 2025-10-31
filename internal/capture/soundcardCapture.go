package capture

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/cmplx"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"

	"github.com/gordonklaus/portaudio"
	"github.com/mjibson/go-dsp/fft"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

type SoundcardCapture struct {
	StreamParams    portaudio.StreamParameters
	SampleRate      int
	BaudRate        float64
	MarkFrequency   float64
	SpaceFrequency  float64
	ChannelNum      int
	BitDepth        int
	BufferSize      int
	stop            chan bool
	sendChannel     chan string
	logger          *log.Logger
	outputChan      chan []byte
	stationCallsign string
	txBuffer        []float32
	preambleFlags   int
	tailTone        []float32
	txMu            sync.Mutex
}

const (
	SampleRate         = 44100
	BaudRate           = 1200
	Tone1200Hz         = 1200.0
	Tone2200Hz         = 2200.0
	BitsPerSample      = 8
	SamplesPerBaud     = SampleRate / BaudRate
	Volume             = 1.0
	TwoPi              = 2.0 * math.Pi
	TailFlags          = 2
	MaxCallsignLength  = 6  // Maximum callsign length in AX.25
	MaxSSID            = 15 // Maximum SSID value in AX.25
)

const defaultTxDelay = 300 * time.Millisecond

var defaultPreambleFlags = preambleFlagsForDelay(defaultTxDelay)

func NewSoundcardCapture(cfg config.Config, outputChan chan []byte, logger *log.Logger) (*SoundcardCapture, error) {
	var (
		channelNum = 1
		bitDepth   = 1
	)

	if err := portaudio.Initialize(); err != nil {
		return nil, err
	}
	defer portaudio.Terminate()

	inputDevice, outputDevice, err := getDevicesByName(cfg.SoundcardInputName, cfg.SoundcardOutputName)
	if err != nil {
		return nil, err
	}

	inputDeviceInfo := portaudio.StreamDeviceParameters{
		Device:   inputDevice,
		Channels: 1,
		Latency:  inputDevice.DefaultLowInputLatency,
	}

	outputDeviceInfo := portaudio.StreamDeviceParameters{
		Device:   outputDevice,
		Channels: 1,
		Latency:  time.Duration(time.Millisecond * 5),
	}

	streamParams := portaudio.StreamParameters{
		Input:           inputDeviceInfo,
		Output:          outputDeviceInfo,
		SampleRate:      float64(SampleRate),
		FramesPerBuffer: 262144,
		Flags:           portaudio.ClipOff,
	}

	sc := &SoundcardCapture{
		StreamParams:    streamParams,
		SampleRate:      SampleRate,
		BaudRate:        BaudRate,
		MarkFrequency:   Tone1200Hz,
		SpaceFrequency:  Tone2200Hz,
		ChannelNum:      channelNum,
		BitDepth:        bitDepth,
		stop:            make(chan bool),
		sendChannel:     make(chan string, 10),
		logger:          logger,
		outputChan:      outputChan,
		stationCallsign: cfg.StationCallsign,
		preambleFlags:   preambleFlagsForDelay(cfg.Transmitter.TxDelay),
		tailTone:        generateTone(Tone1200Hz, cfg.Transmitter.TxTail),
	}

	return sc, nil
}

func Int16ToFloat32(data []int16) []float32 {
	floatData := make([]float32, len(data))
	for i, v := range data {
		floatData[i] = float32(v) / 32768.0
	}
	return floatData
}

func (s *SoundcardCapture) Start() error {
	if err := portaudio.Initialize(); err != nil {
		return err
	}
	defer portaudio.Terminate()

	var audioBuffer []float32

	go func() {
		for msg := range s.sendChannel {
			ax25Packet, err := s.aprsToAx25(msg)
			if err != nil {
				s.logger.Error("Failed to convert APRS frame: ", err)
				continue
			}

			wave, err := s.generateAFSK(ax25Packet)
			if err != nil {
				s.logger.Error("Failed to generate AFSK audio: ", err)
				continue
			}

			s.txMu.Lock()
			s.txBuffer = append(s.txBuffer, wave...)
			s.txMu.Unlock()
		}
	}()

	stream, err := portaudio.OpenStream(s.StreamParams, func(in []float32, out []float32) {
		if len(audioBuffer) != len(in) {
			audioBuffer = make([]float32, len(in))
		}
		copy(audioBuffer, in)

		go func(data []float32) {
			bytes, err := s.float32ToBytes(data)
			if err != nil {
				s.logger.Error("Error converting float32 to bytes: ", err)
				return
			}

			s.outputChan <- bytes
		}(append([]float32(nil), audioBuffer...))

		s.txMu.Lock()
		if len(s.txBuffer) > 0 {
			n := copy(out, s.txBuffer)
			if n < len(s.txBuffer) {
				s.txBuffer = s.txBuffer[n:]
			} else {
				s.txBuffer = s.txBuffer[:0]
			}
			for i := n; i < len(out); i++ {
				out[i] = 0
			}
			s.logger.Debug("Output played to soundcard.")
		} else {
			for i := range out {
				out[i] = 0
			}
		}
		s.txMu.Unlock()
	})
	if err != nil {
		return err
	}
	defer stream.Close()

	if err := stream.Start(); err != nil {
		s.logger.Error("failed to start soundcard: ", err)
	}
	defer stream.Stop()

	for {
		select {
		case <-s.stop:
			return nil
		}
	}
}

func (s *SoundcardCapture) Play(msg string) {
	s.sendChannel <- msg
}

func (s *SoundcardCapture) Stop() {
	s.stop <- true
}

func (s *SoundcardCapture) Type() string {
	return "Soundcard"
}

func (s *SoundcardCapture) float32ToBytes(data []float32) ([]byte, error) {
	buf := make([]byte, len(data)*2)
	littleEndian := isLittleEndian()

	for i, f := range data {
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			f = 0
		}
		if f > 1 {
			f = 1
		} else if f < -1 {
			f = -1
		}

		sample := int16(f * 32767)
		offset := i * 2

		if littleEndian {
			binary.LittleEndian.PutUint16(buf[offset:], uint16(sample))
		} else {
			binary.BigEndian.PutUint16(buf[offset:], uint16(sample))
		}
	}

	return buf, nil
}

func getDevicesByName(inputName, outputName string) (inputDevice, outputDevice *portaudio.DeviceInfo, err error) {
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, nil, err
	}

	if inputName == "" {
		fmt.Println("No device name for input specified, setting to the system default.")
		fmt.Println("Please specify a device name in the config.yml file if you want to select a specific device.")
		fmt.Println("Available devices:")

		for _, device := range devices {
			fmt.Printf("%s\n", device.Name)
		}

		inputDevice, err = portaudio.DefaultInputDevice()
		if err != nil {
			return nil, nil, fmt.Errorf("Error getting default input device: %v", err)
		}
	} else {
		for _, device := range devices {
			if device.Name == inputName {
				inputDevice = device
				break
			}
		}

		if inputDevice == nil {
			fmt.Printf("Input device '%s' not found. Reverting to system default.\n", inputName)
			inputDevice, err = portaudio.DefaultInputDevice()
			if err != nil {
				return nil, nil, fmt.Errorf("Error getting default input device: %v", err)
			}
		}
	}

	if outputName == "" {
		fmt.Println("No device name for output specified, setting to the system default.")
		fmt.Println("Please specify a device name in the config.yml file if you want to select a specific device.")
		fmt.Println("Available devices:")

		for _, device := range devices {
			fmt.Printf("%s\n", device.Name)
		}

		outputDevice, err = portaudio.DefaultOutputDevice()
		if err != nil {
			return nil, nil, fmt.Errorf("Error getting default output device: %v", err)
		}
	} else {
		for _, device := range devices {
			if device.Name == outputName {
				outputDevice = device
				break
			}
		}

		if outputDevice == nil {
			fmt.Printf("Output device '%s' not found. Reverting to system default.\n", inputName)
			outputDevice, err = portaudio.DefaultOutputDevice()
			if err != nil {
				return nil, nil, fmt.Errorf("Error getting default input device: %v", err)
			}
		}
	}

	return inputDevice, outputDevice, nil
}

func (s *SoundcardCapture) aprsToAx25(raw string) ([]byte, error) {
	packet, err := aprs.ParsePacket(raw)
	if err != nil {
		return nil, fmt.Errorf("parse APRS packet: %w", err)
	}

	addresses := make([]string, 0, 2+len(packet.Path))
	addresses = append(addresses, strings.TrimSpace(packet.Dst))
	addresses = append(addresses, strings.TrimSpace(packet.Src))

	for _, path := range packet.Path {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		addresses = append(addresses, path)
	}

	if len(addresses) < 2 {
		return nil, fmt.Errorf("invalid AX.25 address set generated from %q", raw)
	}

	var addr bytes.Buffer
	for i, address := range addresses {
		encoded, err := encodeAX25Address(address, i == len(addresses)-1)
		if err != nil {
			return nil, fmt.Errorf("encode AX.25 address %q: %w", address, err)
		}
		addr.Write(encoded)
	}

	frame := addr.Bytes()
	frame = append(frame, 0x03) // UI frame
	frame = append(frame, 0xF0) // No layer 3 protocol
	frame = append(frame, []byte(packet.Payload)...)

	fcs := computeFCS(frame)
	frame = append(frame, byte(fcs&0xFF), byte((fcs>>8)&0xFF))

	return frame, nil
}

func encodeAX25Address(address string, last bool) ([]byte, error) {
	repeated := strings.HasSuffix(address, "*")
	if repeated {
		address = strings.TrimSuffix(address, "*")
	}

	base := address
	ssid := 0
	if idx := strings.Index(base, "-"); idx != -1 {
		ssidPart := base[idx+1:]
		base = base[:idx]

		if ssidPart != "" {
			parsed, err := strconv.Atoi(ssidPart)
			if err != nil {
				return nil, fmt.Errorf("invalid SSID in %q", address)
			}
			if parsed < 0 || parsed > MaxSSID {
				return nil, fmt.Errorf("SSID out of range in %q", address)
			}
			ssid = parsed
		}
	}

	base = strings.ToUpper(strings.TrimSpace(base))
	if len(base) == 0 || len(base) > MaxCallsignLength {
		return nil, fmt.Errorf("invalid callsign %q", address)
	}

	encoded := make([]byte, 7)
	for i := 0; i < MaxCallsignLength; i++ {
		var c byte = ' '
		if i < len(base) {
			c = base[i]
		}
		encoded[i] = c << 1
	}

	ssidByte := byte(0x60 | ((ssid & 0x0F) << 1))
	if repeated {
		ssidByte |= 0x80
	}
	if last {
		ssidByte |= 0x01
	}
	encoded[6] = ssidByte

	return encoded, nil
}

func computeFCS(frame []byte) uint16 {
	const polynomial = 0x8408
	var fcs uint16 = 0xFFFF

	for _, b := range frame {
		fcs ^= uint16(b)
		for i := 0; i < 8; i++ {
			if fcs&0x0001 != 0 {
				fcs = (fcs >> 1) ^ polynomial
			} else {
				fcs >>= 1
			}
		}
	}

	return ^fcs
}

func (s *SoundcardCapture) generateAFSK(frame []byte) ([]float32, error) {
	if len(frame) == 0 {
		return nil, fmt.Errorf("empty AX.25 frame")
	}

	preamble := s.preambleFlags
	if preamble <= 0 {
		preamble = defaultPreambleFlags
	}

	bits := make([]int, 0, (len(frame)+preamble+TailFlags)*8)
	for i := 0; i < preamble; i++ {
		bits = appendFlag(bits)
	}

	bits = append(bits, bitStuff(frame)...)

	for i := 0; i < TailFlags; i++ {
		bits = appendFlag(bits)
	}

	signal := bitsToAFSK(bits)
	total := append([]float32(nil), signal...)
	if len(s.tailTone) > 0 {
		total = append(total, s.tailTone...)
	}
	completeWave := removeDCOffset(total)
	return normalizeSignal(completeWave), nil
}

// AprsToAx25ForTest exposes aprsToAx25 for test utilities.
func (s *SoundcardCapture) AprsToAx25ForTest(raw string) ([]byte, error) {
	return s.aprsToAx25(raw)
}

// GenerateAFSKForTest exposes generateAFSK for test utilities.
func (s *SoundcardCapture) GenerateAFSKForTest(frame []byte) ([]float32, error) {
	return s.generateAFSK(frame)
}

func appendFlag(bits []int) []int {
	return appendByteLSB(bits, 0x7E)
}

func appendByteLSB(bits []int, value byte) []int {
	for i := 0; i < 8; i++ {
		bits = append(bits, int((value>>uint(i))&0x01))
	}
	return bits
}

func bitStuff(frame []byte) []int {
	bits := make([]int, 0, len(frame)*8)
	ones := 0

	for _, b := range frame {
		for i := 0; i < 8; i++ {
			bit := int((b >> uint(i)) & 0x01)
			bits = append(bits, bit)

			if bit == 1 {
				ones++
				if ones == 5 {
					bits = append(bits, 0)
					ones = 0
				}
			} else {
				ones = 0
			}
		}
	}

	return bits
}

func bitsToAFSK(bits []int) []float32 {
	if len(bits) == 0 {
		return nil
	}

	wave := make([]float32, 0, int(float64(len(bits))*float64(SampleRate)/BaudRate))
	var (
		samplesPerBit = float64(SampleRate) / BaudRate
		remainder     float64
		phase         float64
		currentMark   = true
	)

	for _, bit := range bits {
		if bit == 0 {
			currentMark = !currentMark
		}

		freq := Tone2200Hz
		if currentMark {
			freq = Tone1200Hz
		}

		phaseIncrement := TwoPi * freq / SampleRate
		totalSamples := samplesPerBit + remainder
		sampleCount := int(math.Floor(totalSamples))
		if sampleCount <= 0 {
			sampleCount = 1
		}
		remainder = totalSamples - float64(sampleCount)

		for i := 0; i < sampleCount; i++ {
			wave = append(wave, Volume*float32(math.Sin(phase)))
			phase += phaseIncrement
			if phase >= TwoPi {
				phase -= TwoPi
			}
		}
	}

	return wave
}

func normalizeSignal(signal []float32) []float32 {
	maxAmplitude := float32(0)
	for _, sample := range signal {
		if math.Abs(float64(sample)) > float64(maxAmplitude) {
			maxAmplitude = float32(math.Abs(float64(sample)))
		}
	}
	if maxAmplitude > 0 {
		safetyMargin := float32(0.9) // Scale down the signal slightly
		for i := range signal {
			signal[i] = signal[i] / maxAmplitude * safetyMargin
		}
	}
	return signal
}

func removeDCOffset(signal []float32) []float32 {
	mean := float32(0)
	for _, sample := range signal {
		mean += sample
	}
	mean /= float32(len(signal))

	for i := range signal {
		signal[i] -= mean
	}

	return signal
}

func generateTone(freq float64, duration time.Duration) []float32 {
	if duration <= 0 {
		return nil
	}

	samples := int(math.Ceil(duration.Seconds() * float64(SampleRate)))
	tone := make([]float32, samples)
	phaseIncrement := TwoPi * freq / SampleRate
	var phase float64

	for i := 0; i < samples; i++ {
		tone[i] = Volume * float32(math.Sin(phase))
		phase += phaseIncrement
		if phase >= TwoPi {
			phase -= TwoPi
		}
	}

	return tone
}

func isLittleEndian() bool {
	var i int32 = 0x01020304
	u := (*[4]byte)(unsafe.Pointer(&i))
	return u[0] == 0x04
}

func preambleFlagsForDelay(delay time.Duration) int {
	if delay <= 0 {
		delay = defaultTxDelay
	}

	flags := int(math.Ceil(delay.Seconds() * BaudRate / 8.0))
	if flags < 1 {
		flags = 1
	}
	return flags
}

func (s *SoundcardCapture) displayFFT(signal []float32) {
	// Convert signal to complex128 for FFT processing
	complexSignal := make([]complex128, len(signal))
	for i, v := range signal {
		complexSignal[i] = complex(float64(v), 0)
	}

	// Perform FFT
	fftResult := fft.FFT(complexSignal)

	// Compute the magnitudes of the FFT result
	magnitudes := make(plotter.XYs, len(fftResult)/2)
	for i := 0; i < len(fftResult)/2; i++ {
		magnitudes[i].X = float64(i) * float64(SampleRate) / float64(len(signal))
		magnitudes[i].Y = cmplx.Abs(fftResult[i])
	}

	// Plot the magnitudes
	p := plot.New()

	p.Title.Text = "FFT of AFSK Signal"
	p.X.Label.Text = "Frequency (Hz)"
	p.Y.Label.Text = "Magnitude"

	plotData, err := plotter.NewLine(magnitudes)
	if err != nil {
		s.logger.Error("Failed to create plot data: ", err)
		return
	}

	p.Add(plotData)

	if err := p.Save(10*vg.Inch, 4*vg.Inch, "fft.png"); err != nil {
		s.logger.Error("Failed to save plot: ", err)
		return
	}

	fmt.Println("FFT plot saved to fft.png")
}
