package capture

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/cmplx"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	StreamParams        portaudio.StreamParameters
	SampleRate          int
	inputSampleRate     int
	BaudRate            float64
	MarkFrequency       float64
	SpaceFrequency      float64
	ChannelNum          int
	BitDepth            int
	BufferSize          int
	inputChannels       int
	stop                chan struct{}
	sendChannel         chan string
	logger              *log.Logger
	outputChan          chan []byte
	audioInputChan      chan []float32
	stationCallsign     string
	txBuffer            []float32
	preambleFlags       int
	tailTone            []float32
	txMu                sync.Mutex
	stopOnce            sync.Once
	stream              *portaudio.Stream
	started             bool
	streamMu            sync.Mutex
	useAplay            bool
	txAudioLevel        float32
	receiveEnabled      bool
	receiveDiagnostics  bool
	aplayDevice         string
	droppedInputBuffers atomic.Uint64
	capturedFrames      atomic.Uint64
	processedSamples    atomic.Uint64
	inputSquares        atomic.Uint64
	inputPeak           atomic.Uint64
	decoderBytes        atomic.Uint64
}

const (
	SampleRate        = 44100
	BaudRate          = 1200
	Tone1200Hz        = 1200.0
	Tone2200Hz        = 2200.0
	BitsPerSample     = 8
	SamplesPerBaud    = SampleRate / BaudRate
	Volume            = 1.0
	TwoPi             = 2.0 * math.Pi
	TailFlags         = 2
	MaxCallsignLength = 6  // Maximum callsign length in AX.25
	MaxSSID           = 15 // Maximum SSID value in AX.25
)

const defaultTxDelay = 300 * time.Millisecond

var defaultPreambleFlags = preambleFlagsForDelay(defaultTxDelay)

func NewSoundcardCapture(cfg config.Config, outputChan chan []byte, logger *log.Logger) (*SoundcardCapture, error) {
	var (
		channelNum = 1
		bitDepth   = 1
	)

	sc := &SoundcardCapture{
		SampleRate:         SampleRate,
		inputSampleRate:    SampleRate,
		BaudRate:           BaudRate,
		MarkFrequency:      Tone1200Hz,
		SpaceFrequency:     Tone2200Hz,
		ChannelNum:         channelNum,
		BitDepth:           bitDepth,
		stop:               make(chan struct{}),
		sendChannel:        make(chan string, 10),
		logger:             logger,
		outputChan:         outputChan,
		audioInputChan:     make(chan []float32, 10),
		stationCallsign:    cfg.StationCallsign,
		preambleFlags:      preambleFlagsForDelay(cfg.Transmitter.TxDelay),
		tailTone:           generateTone(Tone1200Hz, cfg.Transmitter.TxTail),
		useAplay:           cfg.Transmitter.UseAplay,
		txAudioLevel:       float32(cfg.Transmitter.AudioLevel),
		receiveEnabled:     cfg.SoundcardCapture || !cfg.Sdr.Enabled,
		receiveDiagnostics: cfg.Multimon.ReceiveDiagnostics,
		aplayDevice:        cfg.SoundcardOutputName,
	}

	if sc.useAplay && sc.aplayDevice == "" {
		sc.aplayDevice = "default"
	}
	if !sc.receiveEnabled && sc.useAplay {
		return sc, nil
	}

	if err := portaudio.Initialize(); err != nil {
		return nil, err
	}
	defer portaudio.Terminate()

	inputDevice, outputDevice, err := getDevicesByName(
		cfg.SoundcardInputName,
		cfg.SoundcardOutputName,
		sc.receiveEnabled,
		!sc.useAplay,
	)
	if err != nil {
		return nil, err
	}

	streamParams := portaudio.StreamParameters{
		SampleRate:      float64(SampleRate),
		FramesPerBuffer: portaudio.FramesPerBufferUnspecified,
		Flags:           portaudio.ClipOff,
	}
	if sc.receiveEnabled {
		inputChannels := min(inputDevice.MaxInputChannels, 2)
		if inputChannels < 1 {
			return nil, fmt.Errorf("input device %q has no capture channels", inputDevice.Name)
		}
		sc.inputChannels = inputChannels
		streamParams.Input = portaudio.StreamDeviceParameters{
			Device:   inputDevice,
			Channels: inputChannels,
			Latency:  inputDevice.DefaultHighInputLatency,
		}
	}
	if !sc.useAplay {
		outputDeviceInfo := portaudio.StreamDeviceParameters{
			Device:   outputDevice,
			Channels: channelNum,
			Latency:  time.Duration(time.Millisecond * 5),
		}
		streamParams.Output = outputDeviceInfo
	}

	sc.StreamParams = streamParams

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
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	if s.started {
		return nil
	}
	if !s.receiveEnabled && s.useAplay {
		s.started = true
		go s.runAplayLoop()
		return nil
	}
	if err := portaudio.Initialize(); err != nil {
		return err
	}

	stream, err := portaudio.OpenStream(s.StreamParams, func(in []float32, out []float32) {
		if s.receiveEnabled {
			if s.receiveDiagnostics && s.inputChannels > 0 {
				s.capturedFrames.Add(uint64(len(in) / s.inputChannels))
			}
			data := append([]float32(nil), in...)
			select {
			case s.audioInputChan <- data:
			case <-s.stop:
			default:
				s.droppedInputBuffers.Add(1)
			}
		}

		if s.useAplay {
			return
		}
		s.txMu.Lock()
		n := copy(out, s.txBuffer)
		if n < len(s.txBuffer) {
			s.txBuffer = s.txBuffer[n:]
		} else {
			s.txBuffer = s.txBuffer[:0]
		}
		for i := n; i < len(out); i++ {
			out[i] = 0
		}
		if n > 0 {
			s.logger.Debug("Output played to soundcard.")
		}
		s.txMu.Unlock()
	})
	if err != nil {
		_ = portaudio.Terminate()
		return err
	}

	if err := stream.Start(); err != nil {
		_ = stream.Close()
		_ = portaudio.Terminate()
		return fmt.Errorf("start sound-card stream: %w", err)
	}
	s.stream = stream
	s.started = true
	streamInfo := stream.Info()
	if streamInfo != nil && streamInfo.SampleRate > 0 {
		s.inputSampleRate = int(math.Round(streamInfo.SampleRate))
	}
	if s.receiveEnabled {
		deviceName := "unknown"
		inputLatency := time.Duration(0)
		if s.StreamParams.Input.Device != nil {
			deviceName = s.StreamParams.Input.Device.Name
		}
		if streamInfo != nil {
			inputLatency = streamInfo.InputLatency
		}
		s.logger.Info("Sound-card receive started: device=", deviceName,
			" channels=", s.inputChannels, " capture-rate=", s.inputSampleRate,
			" input-latency=", inputLatency, " decoder-rate=22050Hz mono")
		go s.runAudioOutputLoop()
		go s.runReceiveDiagnostics()
	}
	if s.useAplay {
		go s.runAplayLoop()
	} else {
		go s.runPortAudioTxLoop()
	}
	return nil
}

func (s *SoundcardCapture) runAudioOutputLoop() {
	resampler := newMultimonResampler(s.inputChannels, s.inputSampleRate)
	for {
		select {
		case <-s.stop:
			return
		case samples := <-s.audioInputChan:
			if s.receiveDiagnostics {
				var squares uint64
				var peak uint64
				for _, sample := range samples {
					amplitude := math.Abs(float64(sample))
					if math.IsNaN(amplitude) || math.IsInf(amplitude, 0) {
						continue
					}
					const metricScale = 1_000_000
					scaled := uint64(amplitude * metricScale)
					squares += scaled * scaled
					if scaled > peak {
						peak = scaled
					}
				}
				s.processedSamples.Add(uint64(len(samples)))
				s.inputSquares.Add(squares)
				for current := s.inputPeak.Load(); peak > current; current = s.inputPeak.Load() {
					if s.inputPeak.CompareAndSwap(current, peak) {
						break
					}
				}
			}
			decoderSamples := resampler.Process(samples)
			data, err := s.float32ToBytes(decoderSamples)
			if err != nil {
				s.logger.Error("Error converting float32 to bytes: ", err)
				continue
			}
			select {
			case s.outputChan <- data:
				if s.receiveDiagnostics {
					s.decoderBytes.Add(uint64(len(data)))
				}
			case <-s.stop:
				return
			}
		}
	}
}

func (s *SoundcardCapture) runReceiveDiagnostics() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			dropped := s.droppedInputBuffers.Swap(0)
			if dropped > 0 {
				s.logger.Warn("Sound-card receive dropped ", dropped, " input buffers in the last 15s; APRS audio may be lost")
			}
			if !s.receiveDiagnostics {
				continue
			}
			frames := s.capturedFrames.Swap(0)
			samples := s.processedSamples.Swap(0)
			squares := s.inputSquares.Swap(0)
			peak := s.inputPeak.Swap(0)
			decoderBytes := s.decoderBytes.Swap(0)
			rms := 0.0
			if samples > 0 {
				rms = math.Sqrt(float64(squares) / float64(samples) / 1_000_000_000_000)
			}
			s.logger.Info("Sound-card receive health: captured-frames=", frames,
				" processed-samples=", samples, " rms=", fmt.Sprintf("%.5f", rms),
				" peak=", fmt.Sprintf("%.5f", float64(peak)/1_000_000),
				" decoder-bytes=", decoderBytes,
				" capture-queue=", len(s.audioInputChan), "/", cap(s.audioInputChan),
				" decoder-queue=", len(s.outputChan), "/", cap(s.outputChan),
				" dropped-buffers=", dropped)
		}
	}
}

type multimonResampler struct {
	channels  int
	inputRate int
	pending   []float32
	position  float64
	inputStep float64
}

func newMultimonResampler(channels, inputRate int) *multimonResampler {
	return &multimonResampler{
		channels:  channels,
		inputRate: inputRate,
		inputStep: float64(inputRate) / float64(SampleRate/2),
	}
}

// Process downmixes and resamples one callback while retaining samples and
// phase needed to make output independent of callback boundaries.
func (r *multimonResampler) Process(samples []float32) []float32 {
	mono := downmixToMono(samples, r.channels)
	if len(mono) == 0 || r.inputRate <= 0 {
		return nil
	}
	if r.inputRate == SampleRate/2 {
		return mono
	}
	r.pending = append(r.pending, mono...)

	// Preserve the existing pair averaging at 44.1 kHz, carrying an odd
	// trailing sample into the next callback rather than dropping it.
	if r.inputRate == SampleRate {
		count := len(r.pending) / 2
		if count == 0 {
			return nil
		}
		resampled := make([]float32, count)
		for i := range resampled {
			resampled[i] = (r.pending[i*2] + r.pending[i*2+1]) / 2
		}
		r.pending = r.pending[count*2:]
		return resampled
	}

	if r.inputStep <= 0 {
		return nil
	}
	resampled := make([]float32, 0, int(float64(len(r.pending))/r.inputStep)+1)
	for r.position+1 < float64(len(r.pending)) {
		left := int(r.position)
		fraction := float32(r.position - float64(left))
		resampled = append(resampled,
			r.pending[left]*(1-fraction)+r.pending[left+1]*fraction)
		r.position += r.inputStep
	}
	consumed := int(r.position)
	if consumed > 0 {
		// Keep the last sample available as the left endpoint for the next
		// interpolated output.
		if consumed >= len(r.pending) {
			consumed = len(r.pending) - 1
		}
		r.pending = r.pending[consumed:]
		r.position -= float64(consumed)
	}
	return resampled
}

func downmixToMono(samples []float32, channels int) []float32 {
	if channels <= 1 {
		return samples
	}
	frameCount := len(samples) / channels
	mono := make([]float32, frameCount)
	for frame := 0; frame < frameCount; frame++ {
		var total float32
		for channel := 0; channel < channels; channel++ {
			total += samples[frame*channels+channel]
		}
		mono[frame] = total / float32(channels)
	}
	return mono
}

func (s *SoundcardCapture) runPortAudioTxLoop() {
	for {
		select {
		case <-s.stop:
			return
		case msg, ok := <-s.sendChannel:
			if !ok {
				return
			}
			msg = strings.TrimSpace(msg)
			if msg == "" {
				continue
			}
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
			if err := dumpWaveForDebug(wave); err != nil {
				s.logger.Warn("Failed to write debug wave: ", err)
			}
			s.txMu.Lock()
			s.txBuffer = append(s.txBuffer, wave...)
			s.txMu.Unlock()
		}
	}
}

func (s *SoundcardCapture) Play(msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}

	select {
	case <-s.stop:
		return
	default:
	}

	select {
	case s.sendChannel <- msg:
	default:
		go func(m string) {
			select {
			case s.sendChannel <- m:
			case <-s.stop:
			}
		}(msg)
	}
}

func (s *SoundcardCapture) Stop() {
	s.stopOnce.Do(func() {
		close(s.stop)
		close(s.sendChannel)
		s.streamMu.Lock()
		defer s.streamMu.Unlock()
		if s.stream != nil {
			if err := s.stream.Stop(); err != nil {
				s.logger.Warn("Error stopping sound-card stream: ", err)
			}
			if err := s.stream.Close(); err != nil {
				s.logger.Warn("Error closing sound-card stream: ", err)
			}
			s.stream = nil
			if err := portaudio.Terminate(); err != nil {
				s.logger.Warn("Error terminating PortAudio: ", err)
			}
		}
	})
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

func getDevicesByName(inputName, outputName string, needInput, needOutput bool) (inputDevice, outputDevice *portaudio.DeviceInfo, err error) {
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, nil, err
	}

	if needInput && inputName == "" {
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
	} else if needInput {
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

	if needOutput && outputName == "" {
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
	} else if needOutput {
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
	return normalizeSignal(completeWave, s.txAudioLevel), nil
}

func (s *SoundcardCapture) runAplayLoop() {
	for {
		select {
		case <-s.stop:
			return
		case msg, ok := <-s.sendChannel:
			if !ok {
				return
			}

			msg = strings.TrimSpace(msg)
			if msg == "" {
				continue
			}

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

			if err := dumpWaveForDebug(wave); err != nil {
				s.logger.Warn("Failed to write debug wave: ", err)
			}

			if err := s.playWithAplay(wave); err != nil {
				s.logger.Error("Failed to transmit with aplay: ", err)
			}
		}
	}
}

func (s *SoundcardCapture) playWithAplay(samples []float32) error {
	file, err := os.CreateTemp("", "go-igate-tx-*.wav")
	if err != nil {
		return fmt.Errorf("create temp wav: %w", err)
	}
	tmpName := file.Name()

	if _, err := file.Write(buildDebugWav(samples)); err != nil {
		file.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp wav: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp wav: %w", err)
	}

	args := []string{"aplay", "-q"}
	if s.aplayDevice != "" {
		args = append(args, "-D", s.aplayDevice)
	}
	args = append(args, tmpName)

	cmd := exec.Command(args[0], args[1:]...)
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("aplay command failed: %w", err)
	}

	return os.Remove(tmpName)
}

func dumpWaveForDebug(samples []float32) error {
	dir := os.Getenv("GO_IGATE_DEBUG_WAV_DIR")
	if dir == "" || len(samples) == 0 {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create debug directory: %w", err)
	}

	filename := filepath.Join(dir, fmt.Sprintf("tx_%d.wav", time.Now().UnixNano()))
	return os.WriteFile(filename, buildDebugWav(samples), 0o644)
}

func buildDebugWav(samples []float32) []byte {
	const (
		audioFormat   = 1
		numChannels   = 1
		bitsPerSample = 16
		blockAlign    = numChannels * bitsPerSample / 8
	)
	byteRate := SampleRate * numChannels * bitsPerSample / 8

	var pcm bytes.Buffer
	for _, sample := range samples {
		if sample > 1 {
			sample = 1
		} else if sample < -1 {
			sample = -1
		}
		value := int16(sample * 32767)
		_ = binary.Write(&pcm, binary.LittleEndian, value)
	}

	dataLen := uint32(pcm.Len())
	riffSize := 4 + 8 + 16 + 8 + dataLen

	var wav bytes.Buffer
	wav.WriteString("RIFF")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(riffSize))
	wav.WriteString("WAVE")
	wav.WriteString("fmt ")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(16))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(audioFormat))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(numChannels))
	_ = binary.Write(&wav, binary.LittleEndian, uint32(SampleRate))
	_ = binary.Write(&wav, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(bitsPerSample))
	wav.WriteString("data")
	_ = binary.Write(&wav, binary.LittleEndian, dataLen)
	wav.Write(pcm.Bytes())

	return wav.Bytes()
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

func normalizeSignal(signal []float32, targetLevel float32) []float32 {
	if targetLevel <= 0 || targetLevel > 1 {
		targetLevel = 0.5
	}

	maxAmplitude := float32(0)
	for _, sample := range signal {
		if math.Abs(float64(sample)) > float64(maxAmplitude) {
			maxAmplitude = float32(math.Abs(float64(sample)))
		}
	}
	if maxAmplitude > 0 {
		for i := range signal {
			signal[i] = signal[i] / maxAmplitude * targetLevel
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
