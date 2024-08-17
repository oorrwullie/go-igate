package capture

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"

	"github.com/gordonklaus/portaudio"
)

type SoundcardCapture struct {
	StreamParams   portaudio.StreamDeviceParameters
	DeviceInfo     *portaudio.DeviceInfo
	SampleRate     int
	BaudRate       int
	MarkFrequency  int
	SpaceFrequency int
	ChannelNum     int
	BitDepth       int
	BufferSize     int
	stop           chan bool
	sendChannel    chan []int16
	logger         *log.Logger
	outputChan     chan []byte
}

func NewSoundcardCapture(cfg config.Config, outputChan chan []byte, logger *log.Logger) (*SoundcardCapture, error) {
	if err := portaudio.Initialize(); err != nil {
		return nil, err
	}

	device, err := selectDeviceByName(cfg.SoundcardName)
	if err != nil {
		return nil, err
	}

	params := portaudio.StreamDeviceParameters{
		Device:   device,
		Channels: 2,
		Latency:  device.DefaultLowOutputLatency,
	}

	sc := &SoundcardCapture{
		StreamParams:   params,
		DeviceInfo:     device,
		SampleRate:     44100,
		BaudRate:       1200,
		MarkFrequency:  1200,
		SpaceFrequency: 2200,
		ChannelNum:     1,
		BitDepth:       2,
		BufferSize:     256,
		stop:           make(chan bool),
		sendChannel:    make(chan []int16, 10),
		logger:         logger,
		outputChan:     outputChan,
	}

	return sc, nil
}

func (s *SoundcardCapture) Start() error {
	var (
		inputChannels  = 1
		outputChannels = 1
	)

	stream, err := portaudio.OpenDefaultStream(inputChannels, outputChannels, float64(s.SampleRate), s.BufferSize,
		func(in, out []int16) {
			go func() {
				s.outputChan <- s.int16ToBytes(in)
			}()

			select {
			case data := <-s.sendChannel:
				copy(out, data)
			default:
				for i := range out {
					out[i] = 0
				}
			}
		})
	if err != nil {
		return err
	}

	go func() {
		defer stream.Close()

		if err := stream.Start(); err != nil {
			s.logger.Error("failed to start soundcard: ", err)
		}

		defer stream.Stop()

		select {
		case <-s.stop:
			return
		}
	}()

	return nil
}

func (s *SoundcardCapture) Play(msg []byte) error {
	binMsg, err := s.bytesToInt16(msg)
	if err != nil {
		return err
	}

	s.sendChannel <- binMsg

	return nil
}

func (s *SoundcardCapture) processAudio(out [][]float32, signal []byte) {
	for i := range out[0] {
		if len(signal) <= 0 {
			out[0][i] = 0.0
			out[1][i] = 0.0
			continue
		}

		sample := int16(signal[0]) | int16(signal[1])<<8
		value := float32(sample) / 32768.0
		out[0][i] = value
		out[1][i] = value
		signal = signal[s.BitDepth:]
	}
}

func (s *SoundcardCapture) int16ToBytes(data []int16) []byte {
	buf := make([]byte, len(data)*2)
	for i, v := range data {
		buf[i*2] = byte(v)
		buf[i*2+1] = byte(v >> 8)
	}
	return buf
}

func (s *SoundcardCapture) bytesToInt16(data []byte) ([]int16, error) {
	if len(data)%2 != 0 {
		return nil, fmt.Errorf("data length is not even")
	}

	int16s := make([]int16, len(data)/2)
	err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &int16s)
	if err != nil {
		return nil, err
	}

	return int16s, nil
}

func (s *SoundcardCapture) Stop() {
	s.stop <- true
	portaudio.Terminate()
}

func (s *SoundcardCapture) Type() string {
	return "Soundcard"
}

func selectDeviceByName(name string) (*portaudio.DeviceInfo, error) {
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, err
	}

	if name == "" {
		fmt.Println("No device name specified, defaulting to the first available device.")
		fmt.Println("Please specify a device name in the config.yml file if you want to select a specific device.")
		fmt.Println("Available devices:")

		for _, device := range devices {
			fmt.Printf("%s\n", device.Name)
		}

		return devices[0], nil
	}

	for _, device := range devices {
		if device.Name == name {
			return device, nil
		}
	}

	fmt.Printf("Device named '%s' not found, defaulting to the first available device.\n", name)
	return devices[0], nil
}
