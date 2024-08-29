package capture

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/cmplx"
	"strings"
	"time"
	"unsafe"

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
}

const (
	SampleRate     = 44100
	BaudRate       = 1200
	Tone1200Hz     = 1200.0
	Tone2200Hz     = 2200.0
	BitsPerSample  = 8
	SamplesPerBaud = SampleRate / BaudRate
	Volume         = 0.5
	TwoPi          = 2.0 * math.Pi
	VOXFrequency   = 1000.0
	VOXDuration    = 0.1
	DelayDuration  = 0.4
)

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

	audioBuffer := make([]float32, s.BufferSize)

	stream, err := portaudio.OpenStream(s.StreamParams, func(in []float32, out []float32) {
		copy(audioBuffer, in)

		go func() {
			bytes, err := s.float32ToBytes(audioBuffer)
			if err != nil {
				s.logger.Error("Error converting float32 to bytes: ", err)
			}

			s.outputChan <- bytes
		}()

		select {
		case msg := <-s.sendChannel:
			ax25Packet := s.aprsToAx25(msg)
			wave := generateAFSK(ax25Packet)

			// s.displayFFT(wave)
			n := copy(out, wave)

			for i := n; i < len(out); i++ {
				out[i] = 0
			}
			s.logger.Debug("Output played to soundcard.")
		}
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
	buf := new(bytes.Buffer)

	for _, f := range data {
		if isLittleEndian() {
			if err := binary.Write(buf, binary.LittleEndian, f); err != nil {
				return nil, err
			}
		} else {
			if err := binary.Write(buf, binary.BigEndian, f); err != nil {
				return nil, err
			}
		}
	}

	return buf.Bytes(), nil
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

func (s *SoundcardCapture) aprsToAx25(aprs string) []byte {
	parts := strings.Split(aprs, ">")
	if len(parts) != 2 {
		s.logger.Error("Invalid APRS string format")
		return nil
	}

	source := parts[0]
	rest := strings.Split(parts[1], ":")
	destination := strings.Split(rest[0], ",")[0]
	viaPath := strings.Split(rest[0], ",")[1:]
	infoField := rest[1]

	var buffer bytes.Buffer
	buffer.WriteByte(0x7E) // Start flag
	buffer.Write(encodeCallsign(destination))
	buffer.Write(encodeCallsign(source))

	for i, via := range viaPath {
		last := (i == len(viaPath)-1)
		buffer.Write(encodeCallsign(via, last))
	}

	buffer.WriteByte(0x03) // Control field
	buffer.WriteByte(0xF0) // Protocol ID
	buffer.WriteString(infoField)

	frame := buffer.Bytes()
	fcs := computeFCS(frame[1:]) // Compute FCS (Frame Check Sequence)

	// Append FCS in the correct byte order
	if isLittleEndian() {
		binary.Write(&buffer, binary.LittleEndian, fcs)
	} else {
		binary.Write(&buffer, binary.BigEndian, fcs)
	}
	buffer.WriteByte(0x7E) // End flag

	return buffer.Bytes()
}

func encodeCallsign(callsign string, last ...bool) []byte {
	callsign = strings.ToUpper(callsign)
	addr := make([]byte, 7)
	for i := 0; i < 6 && i < len(callsign); i++ {
		addr[i] = callsign[i] << 1
	}
	if len(callsign) < 6 {
		addr[len(callsign)] = ' ' << 1
	}
	ssid := 0
	if len(callsign) > 6 {
		ssid = int(callsign[6] - '0')
	}
	addr[6] = byte((ssid&0x0F)<<1 | 0x60)
	if len(last) > 0 && last[0] {
		addr[6] |= 0x01
	}
	return addr
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

func generateAFSK(packet []byte) []float32 {
	samplesPerBit := int(SampleRate / BaudRate)
	signal := make([]float32, 0, len(packet)*8*samplesPerBit)
	currentPhase := 0.0

	// Constants for phase increments
	phaseIncrement1200 := 2.0 * math.Pi * Tone1200Hz / SampleRate
	phaseIncrement2200 := 2.0 * math.Pi * Tone2200Hz / SampleRate

	for _, byteVal := range packet {
		for i := 0; i < 8; i++ {
			bit := (byteVal >> (7 - i)) & 0x01
			phaseIncrement := phaseIncrement2200
			if bit == 1 {
				phaseIncrement = phaseIncrement1200
			}

			for j := 0; j < samplesPerBit; j++ {
				sample := float32(math.Sin(currentPhase))
				signal = append(signal, sample*Volume)
				currentPhase = math.Mod(currentPhase+phaseIncrement, TwoPi)
			}
		}
	}

	voxTone := generateVoxTone()
	wave := append(voxTone, signal...)

	completeWave := removeDCOffset(wave)
	limitedWave := applyLimiter(completeWave, 0.9)

	return normalizeSignal(limitedWave)
}

func generateVoxTone() []float32 {
	samples := int(VOXDuration * SampleRate)
	voxSignal := make([]float32, samples)
	phaseIncrement := TwoPi * VOXFrequency / SampleRate
	var phase float64

	for i := 0; i < samples; i++ {
		voxSignal[i] = Volume * float32(math.Sin(phase))
		phase = math.Mod(phase+phaseIncrement, TwoPi)
	}

	silentDelay := generateSilentDelay()

	voxSignal = append(voxSignal, silentDelay...)

	return voxSignal
}

func generateSilentDelay() []float32 {
	samples := int(DelayDuration * SampleRate)
	silentSignal := make([]float32, samples)
	return silentSignal
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

func applyLimiter(signal []float32, threshold float32) []float32 {
	for i, sample := range signal {
		if sample > threshold {
			signal[i] = threshold
		} else if sample < -threshold {
			signal[i] = -threshold
		}
	}
	return signal
}

func isLittleEndian() bool {
	var i int32 = 0x01020304
	u := (*[4]byte)(unsafe.Pointer(&i))
	return u[0] == 0x04
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
