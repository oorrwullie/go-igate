package capture

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
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

type AX25Frame struct {
	Destination [7]byte
	Source      [7]byte
	Digipeaters [7]byte
	Control     byte
	ProtocolID  byte
	Info        []byte
}

func NewSoundcardCapture(cfg config.Config, outputChan chan []byte, logger *log.Logger) (*SoundcardCapture, error) {
	var (
		sampleRate     = 44100
		baudRate       = 1200.0
		markFrequency  = 1200.0
		spaceFrequency = 2200.0
		channelNum     = 1
		bitDepth       = 1
		bufferSize     = 376320
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

func (s *SoundcardCapture) Start() error {
	if err := portaudio.Initialize(); err != nil {
		return err
	}
	defer portaudio.Terminate()

	audioBuffer := make([]float32, s.BufferSize)

	stream, err := portaudio.OpenStream(s.StreamParams, func(in, out []float32) {
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
			// audioData := s.EncodeAudio(msg)
			frame := s.CreateAX25FrameFromAPRS(msg)
			bitstream := frame.ToBitstream()
			nrziBitstream := frame.NRZIEncode(bitstream)
			audioData := frame.ModulateAFSK(nrziBitstream)

			n := copy(out, audioData)

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

func (s *SoundcardCapture) CreateAX25FrameFromAPRS(msg string) *AX25Frame {
	parts := strings.Split(msg, ">")
	if len(parts) != 2 {
		s.logger.Error("Invalid APRS message: %v", msg)
	}

	sourceDest := strings.Split(parts[0], ":")
	if len(sourceDest) != 2 {
		s.logger.Error("Invalid source/destination in APRS message: %v", parts[0])
		return &AX25Frame{}
	}

	destination := sourceDest[0]
	source := sourceDest[1]

	// Extract the info part from the message
	info := strings.TrimSpace(parts[1])

	// Prepare the destination, source, and digipeater addresses
	dstAddr := callsignToAddress(destination)
	srcAddr := callsignToAddress(source)

	// Prepare the digipeater address
	var digipeaterAddr [7]byte
	if s.stationCallsign != "" {
		digipeaterAddr = callsignToAddress(s.stationCallsign)
	}

	// Create the frame
	return &AX25Frame{
		Destination: dstAddr,
		Source:      srcAddr,
		Digipeaters: digipeaterAddr,
		Control:     0x03, // UI-frame
		ProtocolID:  0xF0, // No layer 3 protocol
		Info:        []byte(info),
	}
}

func callsignToAddress(callsign string) [7]byte {
	var addr [7]byte
	copy(addr[:], callsign)
	for i := 0; i < 6; i++ {
		addr[i] <<= 1
	}
	addr[6] = 0b01100001
	return addr
}

func (frame *AX25Frame) ToBitstream() []int {
	var bitstream []int

	for _, b := range frame.Destination {
		bitstream = append(bitstream, frame.byteToBits(b)...)
	}

	for _, b := range frame.Source {
		bitstream = append(bitstream, frame.byteToBits(b)...)
	}

	for _, b := range frame.Digipeaters {
		bitstream = append(bitstream, frame.byteToBits(b)...)
	}

	bitstream = append(bitstream, frame.byteToBits(frame.Control)...)
	bitstream = append(bitstream, frame.byteToBits(frame.ProtocolID)...)

	for _, b := range frame.Info {
		bitstream = append(bitstream, frame.byteToBits(b)...)
	}

	bitstream = append(bitstream, []int{0, 1, 1, 1, 1, 1, 1, 0}...)

	return bitstream
}

func (frame *AX25Frame) byteToBits(b byte) []int {
	bits := make([]int, 8)
	for i := 0; i < 8; i++ {
		bits[i] = int((b >> (7 - i)) & 1)
	}
	return bits
}

// Apply NRZI encoding to the bitstream
func (frame *AX25Frame) NRZIEncode(bitstream []int) []int {
	var nrzi []int
	currentState := 1
	for _, bit := range bitstream {
		if bit == 1 {
			currentState = currentState ^ 1 // Toggle the state
		}
		nrzi = append(nrzi, currentState)
	}
	return nrzi
}

// Modulate the NRZI-encoded bitstream into AFSK audio
func (frame *AX25Frame) ModulateAFSK(bitstream []int) []float32 {
	sampleRate := 44100
	bitDuration := float64(sampleRate) / 1200
	var audioTrigger []float32
	var audioMsg []float32
	var audio []float32

	// generate VOX trigger tone
	duration := time.Millisecond * 400
	numSamples := int(float64(sampleRate) * duration.Seconds())
	for i := 0; i < numSamples; i++ {
		audioTrigger = append(audioTrigger, float32(0.8*math.Sin(2*math.Pi*1000.0*float64(i)/float64(sampleRate))))
	}

	for _, bit := range bitstream {
		frequency := 1200.0
		if bit == 0 {
			frequency = 2200.0
		}
		for i := 0; i < int(bitDuration); i++ {
			audioMsg = append(audioMsg, float32(0.8*math.Sin(2*math.Pi*frequency*float64(i)/float64(sampleRate))))
		}
	}

	audio = append(audio, audioTrigger...)
	audio = append(audio, audioMsg...)

	return audio
}
