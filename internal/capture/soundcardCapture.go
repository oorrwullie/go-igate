package capture

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"testing"
	"unsafe"

	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"

	"github.com/gordonklaus/portaudio"
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
	MarkFrequency  = 1200.0
	SpaceFrequency = 2200.0
	SamplesPerBaud = SampleRate / BaudRate
	Amplitude      = 0.99
	TwoPi          = 2 * math.Pi
	VOXFrequency   = 1000.0
	VOXDuration    = 0.1
	DelayDuration  = 0.25
)

func NewSoundcardCapture(cfg config.Config, outputChan chan []byte, logger *log.Logger) (*SoundcardCapture, error) {
	var (
		sampleRate     = 44100
		baudRate       = 1200.0
		markFrequency  = 1200.0
		spaceFrequency = 2200.0
		channelNum     = 1
		bitDepth       = 1
		//bufferSize     = 512320
		bufferSize = 376320
		//bufferSize = 74910
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
		Latency:  outputDevice.DefaultLowOutputLatency,
	}

	streamParams := portaudio.StreamParameters{
		Input:           inputDeviceInfo,
		Output:          outputDeviceInfo,
		SampleRate:      float64(sampleRate),
		FramesPerBuffer: bufferSize,
		Flags:           portaudio.NoFlag,
	}

	sc := &SoundcardCapture{
		StreamParams:    streamParams,
		SampleRate:      sampleRate,
		BaudRate:        baudRate,
		MarkFrequency:   markFrequency,
		SpaceFrequency:  spaceFrequency,
		ChannelNum:      channelNum,
		BitDepth:        bitDepth,
		BufferSize:      bufferSize,
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
			src, dst, digi, info := parseAprsString(msg)
			frame := buildAx25Frame(dst, src, digi, info)
			wave := ax25ToAFSK(frame)

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
	isLittleEndian := func() bool {
		var i int32 = 0x01020304
		u := (*[4]byte)(unsafe.Pointer(&i))
		return u[0] == 0x04
	}()

	for _, f := range data {
		if isLittleEndian {
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

func encodeCallsign(callsign string, ssid int) []byte {
	encoded := make([]byte, 7)
	for i := 0; i < 6; i++ {
		if i < len(callsign) {
			encoded[i] = callsign[i] << 1
		} else {
			encoded[i] = ' ' << 1
		}
	}
	encoded[6] = byte((ssid << 1) | 0x60) // Set the 0x60 to indicate an end-of-address flag
	return encoded
}

func crc16Ccitt(data []byte) uint16 {
	var crc uint16 = 0xFFFF
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

func buildAx25Frame(destination, source string, digipeaters []string, infoField string) []byte {
	frame := bytes.Buffer{}

	destCallsign, destSsid := parseCallsign(destination)
	frame.Write(encodeCallsign(destCallsign, destSsid))

	sourceCallsign, sourceSsid := parseCallsign(source)
	frame.Write(encodeCallsign(sourceCallsign, sourceSsid))

	for i, digipeater := range digipeaters {
		digiCallsign, digiSsid := parseCallsign(digipeater)
		encodedDigi := encodeCallsign(digiCallsign, digiSsid)
		if i == len(digipeaters)-1 {
			encodedDigi[6] |= 0x01 // Mark the last digipeater
		}
		frame.Write(encodedDigi)
	}

	frame.WriteByte(0x03)

	frame.WriteByte(0xF0)

	frame.WriteString(infoField)

	fcs := crc16Ccitt(frame.Bytes())
	binary.Write(&frame, binary.LittleEndian, fcs)

	return frame.Bytes()
}

func parseCallsign(callsign string) (string, int) {
	parts := strings.Split(callsign, "-")
	if len(parts) == 2 {
		return parts[0], int(parts[1][0] - '0')
	}
	return callsign, 0
}

func parseAprsString(aprsString string) (string, string, []string, string) {
	parts := strings.Split(aprsString, ">")
	source := parts[0]

	rest := strings.Split(parts[1], ":")
	pathParts := strings.Split(rest[0], ",")
	destination := pathParts[0]

	var digipeaters []string
	if len(pathParts) > 1 {
		digipeaters = pathParts[1:]
	}

	infoField := rest[1]
	return source, destination, digipeaters, infoField
}

// Unit test for CRC-16-CCITT calculation
func TestCrc16Ccitt(t *testing.T) {
	data := []byte("123456789")
	expectedCrc := uint16(0x29B1)
	calculatedCrc := crc16Ccitt(data)
	if calculatedCrc != expectedCrc {
		t.Errorf("CRC-16-CCITT calculation failed, expected %X but got %X", expectedCrc, calculatedCrc)
	}
}

func ax25ToAFSK(frame []byte) []float32 {
	var afskSignal []float32
	var phase float64
	markIncrement := TwoPi * MarkFrequency / SampleRate
	spaceIncrement := TwoPi * SpaceFrequency / SampleRate

	for _, byteValue := range frame {
		for bit := 0; bit < 8; bit++ {
			if byteValue&(0x80>>bit) != 0 {
				// Mark (1200 Hz)
				for i := 0; i < SamplesPerBaud; i++ {
					afskSignal = append(afskSignal, Amplitude*float32(math.Sin(phase)))
					phase += markIncrement
					if phase >= TwoPi {
						phase -= TwoPi
					}
				}
			} else {
				// Space (2200 Hz)
				for i := 0; i < SamplesPerBaud; i++ {
					afskSignal = append(afskSignal, Amplitude*float32(math.Sin(phase)))
					phase += spaceIncrement
					if phase >= TwoPi {
						phase -= TwoPi
					}
				}
			}
		}
	}

	voxTone := generateVoxTone()

	completeWave := append(voxTone, afskSignal...)

	return completeWave
}

func generateVoxTone() []float32 {
	samples := int(VOXDuration * SampleRate)
	voxSignal := make([]float32, samples)
	phaseIncrement := TwoPi * VOXFrequency / SampleRate
	var phase float64

	for i := 0; i < samples; i++ {
		voxSignal[i] = Amplitude * float32(math.Sin(phase))
		phase += phaseIncrement
		if phase >= TwoPi {
			phase -= TwoPi
		}
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
