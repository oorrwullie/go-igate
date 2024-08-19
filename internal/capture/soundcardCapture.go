package capture

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"time"
	"unsafe"

	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"

	"github.com/gordonklaus/portaudio"
)

type SoundcardCapture struct {
	StreamParams   portaudio.StreamParameters
	SampleRate     int
	BaudRate       float64
	MarkFrequency  float64
	SpaceFrequency float64
	ChannelNum     int
	BitDepth       int
	BufferSize     int
	stop           chan bool
	sendChannel    chan string
	logger         *log.Logger
	outputChan     chan []byte
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
		StreamParams:   streamParams,
		SampleRate:     sampleRate,
		BaudRate:       baudRate,
		MarkFrequency:  markFrequency,
		SpaceFrequency: spaceFrequency,
		ChannelNum:     channelNum,
		BitDepth:       bitDepth,
		BufferSize:     bufferSize,
		stop:           make(chan bool),
		sendChannel:    make(chan string, 10),
		logger:         logger,
		outputChan:     outputChan,
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
			audioData := s.EncodeAudio(msg)
			n := copy(out, audioData)

			for i := n; i < len(out); i++ {
				out[i] = 0
			}
		default:
			for i := range out {
				out[i] = 0
			}
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

func (s *SoundcardCapture) EncodeAudio(packet string) []float32 {
	bitstream := s.convertToNRZI(s.packetToBitstream(packet))

	return s.modulateAFSK(bitstream)
}

func (s *SoundcardCapture) convertToNRZI(bitstream []int) []int {
	var nrzi []int
	currentState := 1
	for _, bit := range bitstream {
		if bit == 1 {
			currentState = currentState ^ 1
		}
		nrzi = append(nrzi, currentState)
	}
	return nrzi
}

func (s *SoundcardCapture) modulateAFSK(bitstream []int) []float32 {
	bitDuration := float64(s.SampleRate) / s.BaudRate
	var audio []float32

	// generate VOX trigger tone
	duration := time.Millisecond * 500
	numSamples := int(float64(s.SampleRate) * duration.Seconds())
	for i := 0; i < numSamples; i++ {
		audio = append(audio, float32(0.8*math.Sin(2*math.Pi*1000.0*float64(i)/float64(s.SampleRate))))
	}

	for _, bit := range bitstream {
		frequency := s.MarkFrequency
		if bit == 0 {
			frequency = s.SpaceFrequency
		}
		for i := 0; i < int(bitDuration); i++ {
			audio = append(audio, float32(math.Sin(2*math.Pi*frequency*float64(i)/float64(s.SampleRate))))
		}
	}
	return audio
}

func (s *SoundcardCapture) packetToBitstream(packet string) []int {
	var bitstream []int
	for _, char := range packet {
		for i := 7; i >= 0; i-- {
			bitstream = append(bitstream, int((char>>i)&1))
		}
	}
	return bitstream
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
