package capture

/*
#cgo LDFLAGS: -lax25
#include <ax25.h>
#include <stdlib.h>
#include <string.h>

void encode_callsign(char *callsign, ax25_address *ax25_addr) {
	ax25_aton_entry(callsign, ax25_addr);
}

unsigned int crc16(unsigned char *data, int len) {
   return ax25_compute_fcs(len, data);
}
*/
import "C"
import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unsafe"

	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"

	"github.com/gordonklaus/portaudio"
)

type SoundcardCapture struct {
	StreamParams    portaudio.StreamParameters
	ChannelNum      int
	BitDepth        int
	BufferSize      int
	stop            chan bool
	sendChannel     chan []float32
	logger          *log.Logger
	outputChan      chan []byte
	doneChan        chan bool
	stationCallsign string
}

const (
	SampleRate     = 44100
	BaudRate       = 1200
	MarkFrequency  = 1200.0
	SpaceFrequency = 2200.0
	SamplesPerBaud = SampleRate / BaudRate
	Amplitude      = 1.5
	TwoPi          = 2 * math.Pi
	VOXFrequency   = 1000.0
	VOXDuration    = 0.2
	DelayDuration  = 0.1
)

func NewSoundcardCapture(cfg config.Config, outputChan chan []byte, logger *log.Logger) (*SoundcardCapture, error) {
	var (
		channelNum = 1
		bitDepth   = 1
		//bufferSize     = 512320
		//bufferSize = 376320
		//bufferSize = 74910
		bufferSize = 262144
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
		Latency:  inputDevice.DefaultHighInputLatency,
	}

	outputDeviceInfo := portaudio.StreamDeviceParameters{
		Device:   outputDevice,
		Channels: 1,
		Latency:  outputDevice.DefaultHighOutputLatency,
	}

	streamParams := portaudio.StreamParameters{
		Input:           inputDeviceInfo,
		Output:          outputDeviceInfo,
		SampleRate:      float64(SampleRate),
		FramesPerBuffer: bufferSize,
		Flags:           portaudio.NoFlag,
	}

	sc := &SoundcardCapture{
		StreamParams:    streamParams,
		ChannelNum:      channelNum,
		BitDepth:        bitDepth,
		BufferSize:      bufferSize,
		stop:            make(chan bool),
		sendChannel:     make(chan []float32, 10),
		logger:          logger,
		outputChan:      outputChan,
		stationCallsign: cfg.StationCallsign,
		doneChan:        make(chan bool),
	}

	return sc, nil
}

func (s *SoundcardCapture) Start() error {
	if err := portaudio.Initialize(); err != nil {
		return err
	}
	defer portaudio.Terminate()

	stream, err := portaudio.OpenStream(s.StreamParams, s.audioCallback)
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

func (s *SoundcardCapture) audioCallback(inputBuffer, outputBuffer []float32) {
	audioBuffer := make([]float32, s.BufferSize)
	copy(audioBuffer, inputBuffer)

	go func() {
		bytes, err := s.float32ToBytes(audioBuffer)
		if err != nil {
			s.logger.Error("Error converting float32 to bytes: ", err)
		}

		s.outputChan <- bytes
	}()

	select {
	case wave := <-s.sendChannel:
		for i := range outputBuffer {
			if i < len(wave) {
				outputBuffer[i] = wave[i]
			} else {
				outputBuffer[i] = 0
			}
		}
	case <-s.doneChan:
		for i := range outputBuffer {
			outputBuffer[i] = 0
		}
	default:
		for i := range outputBuffer {
			outputBuffer[i] = 0
		}
	}
}

func (s *SoundcardCapture) Play(msg string) {
	s.doneChan <- false
	src, dst, digi, info := parseAprsString(msg)
	frame := s.buildAx25Frame(dst, src, digi, info)
	wave := s.ax25ToAFSK(frame)

	s.sendChannel <- wave
	// time.Sleep(time.Duration(len(wave)) * time.Second / SampleRate)
	s.doneChan <- true
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

func (s *SoundcardCapture) buildAx25Frame(destination, source string, digipeaters []string, infoField string) []byte {
	frame := bytes.Buffer{}

	destCallsign := C.CString(destination)
	defer C.free(unsafe.Pointer(destCallsign))
	var destAddr C.ax25_address
	C.encode_callsign(destCallsign, &destAddr)
	frame.Write(C.GoBytes(unsafe.Pointer(&destAddr), C.sizeof_ax25_address))

	srcCallsign := C.CString(source)
	defer C.free(unsafe.Pointer(srcCallsign))
	var srcAddr C.ax25_address
	C.encode_callsign(srcCallsign, &srcAddr)
	frame.Write(C.GoBytes(unsafe.Pointer(&srcAddr), C.sizeof_ax25_address))

	for _, digi := range digipeaters {
		digiCallsign := C.CString(digi)
		defer C.free(unsafe.Pointer(digiCallsign))
		var digiAddr C.ax25_address
		C.encode_callsign(digiCallsign, &digiAddr)
		frame.Write(C.GoBytes(unsafe.Pointer(&digiAddr), C.sizeof_ax25_address))
	}

	control := byte(0x03)
	protocol := byte(0xF0)
	frame.WriteByte(control)
	frame.WriteByte(protocol)
	frame.WriteString(infoField)

	crc := C.crc16((*C.uchar)(unsafe.Pointer(&frame.Bytes()[0])), C.int(frame.Len()))
	crcBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(crcBytes, uint16(crc))
	frame.Write(crcBytes)

	return frame.Bytes()
}

func (s *SoundcardCapture) ax25ToAFSK(frame []byte) []float32 {
	var afskSignal []float32
	var phase float64
	markIncrement := TwoPi * s.MarkFrequency / float64(s.SampleRate)
	spaceIncrement := TwoPi * s.SpaceFrequency / float64(s.SampleRate)

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

	// Prepend a VOX tone if necessary
	voxTone := s.generateVoxTone()

	// Append the AFSK signal to the VOX tone
	completeWave := append(voxTone, afskSignal...)

	return completeWave
}

func parseAprsString(s string) (source string, destination string, digipeaters []string, infoField string) {
	// Assuming the standard format for an APRS frame
	parts := strings.Split(s, ">")
	if len(parts) < 2 {
		return
	}
	source = parts[0]

	parts = strings.Split(parts[1], ":")
	if len(parts) < 2 {
		return
	}
	destinationAndDigipeaters := strings.Split(parts[0], ",")
	destination = destinationAndDigipeaters[0]

	if len(destinationAndDigipeaters) > 1 {
		digipeaters = destinationAndDigipeaters[1:]
	}

	infoField = parts[1]

	return
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
