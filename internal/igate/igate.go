package igate

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/cache"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/tarm/serial"
)

type (
	IGate struct {
		cfg          config.Config
		multimonChan chan []byte
		aprsisChan   chan string
		txChan       chan string
		Stop         chan bool
		Aprsis       *aprs.AprsIs
		Logger       *log.Logger
		cache        *cache.Cache
		enableTx     bool
	}
)

const minPacketSize = 35

func New(cfg config.Config, enableTx bool, aprsis *aprs.AprsIs, logger *log.Logger) (*IGate, error) {
	ig := &IGate{
		cfg:          cfg,
		multimonChan: make(chan []byte),
		aprsisChan:   make(chan string),
		txChan:       make(chan string),
		Stop:         make(chan bool),
		Aprsis:       aprsis,
		Logger:       logger,
		cache:        cache.NewCache(1000, 10000, ".cache.json"),
		enableTx:     enableTx,
	}

	return ig, nil
}

func (i *IGate) Run() error {
	err := i.startSDR()
	if err != nil {
		return fmt.Errorf("Error starting SDR: %v", err)
	}

	err = i.startMultimon()
	if err != nil {
		return fmt.Errorf("Error starting multimon: %v", err)
	}

	err = i.startBeacon()
	if err != nil {
		return fmt.Errorf("Error starting beacon: %v", err)
	}

	if i.enableTx {
		err = i.startTx()
		if err != nil {
			return fmt.Errorf("Error starting TX: %v", err)
		}
	}

	i.listenForAprsMessages()

	return nil
}

func (i *IGate) startSDR() error {
	requiredArgs := []string{
		"-f",
		i.cfg.Sdr.Frequency,
		"-s",
		i.cfg.Sdr.SampleRate,
		"-l",
		i.cfg.Sdr.SquelchLevel,
		"-g",
		i.cfg.Sdr.Gain,
		"-p",
		i.cfg.Sdr.PpmError,
	}

	if i.cfg.Sdr.Device != "" && i.cfg.Sdr.Device != "0" {
		i.Logger.Info("Using device ", i.cfg.Sdr.Device)
		requiredArgs = append(requiredArgs, "-d", i.cfg.Sdr.Device)
	}

	userArgs := strings.Fields(i.cfg.Sdr.AdditionalFlags)
	args := append(requiredArgs, userArgs...)
	args = append(args, "-")
	cmd := exec.Command("rtl_fm", args...)

	out, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("Error reading rtl_fm stdout: %s", err.Error())
	}

	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("Error starting rtl_fm: %v", err)
	}

	go func() {
		buf := make([]byte, 4096)

		for {
			n, err := out.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				i.Logger.Error("Error reading rtl_fm stdout:", err)
				return
			}

			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				i.multimonChan <- data
			}
		}
	}()

	go func() {
		for {
			select {
			case <-i.Stop:
				cmd.Process.Kill()
				return
			}
		}
	}()

	return nil
}

func (i *IGate) startMultimon() error {
	go func() {
		requiredArgs := []string{
			"-a",
			"AFSK1200",
			"-A",
			"-t",
			"raw",
		}

		userArgs := strings.Fields(i.cfg.Multimon.AdditionalFlags)
		args := append(requiredArgs, userArgs...)
		args = append(args, "-")
		cmd := exec.Command("multimon-ng", args...)

		inPipe, err := cmd.StdinPipe()
		if err != nil {
			i.Logger.Error("Error opening multimon-ng stdin:", err)
			return
		}

		outPipe, err := cmd.StdoutPipe()
		if err != nil {
			i.Logger.Error("Error opening multimon-ng stdout: ", err)
			return
		}

		err = cmd.Start()
		if err != nil {
			i.Logger.Error("Error starting multimon-ng: ", err)
			return
		}

		go func() {
			for {
				select {
				case <-i.Stop:
					cmd.Process.Kill()
					i.Logger.Info("Stopping multimon-ng process")
					return
				}
			}
		}()

		go func(in io.WriteCloser) {
			defer in.Close()

			for data := range i.multimonChan {
				_, err := in.Write(data)
				if err != nil {
					i.Logger.Error("Error writing to multimon-ng: ", err)
				}
			}
		}(inPipe)

		go func(out io.ReadCloser) {
			scanner := bufio.NewScanner(out)
			for scanner.Scan() {
				msg := scanner.Text()
				if exists := i.cache.Set(msg, time.Now()); !exists {
					i.Logger.Info("packet received: ", msg)

					i.aprsisChan <- msg
				}
			}

			if err := scanner.Err(); err != nil {
				i.Logger.Error("Error reading from multimon-ng: ", err)
			}

		}(outPipe)
	}()

	return nil
}

func (i *IGate) listenForAprsMessages() {
	for {
		select {
		case <-i.Stop:
			return
		case msg := <-i.aprsisChan:
			if len(msg) < minPacketSize {
				i.Logger.Error("Packet too short: ", msg)
				continue
			}

			packet, err := aprs.ParsePacket(msg)
			if err != nil {
				i.Logger.Error(err, "Could not parse APRS packet: ", msg)
				continue
			}

			fmt.Printf("APRS packet type: %v\n", packet.Type().String())
			fmt.Printf("ForwardToAprsIs: %v\n", packet.Type().ForwardToAprsIs())
			fmt.Printf("IsAckMessage: %v\n", packet.IsAckMessage())
			if !packet.IsAckMessage() && packet.Type().ForwardToAprsIs() {
				fmt.Println("It should be sending the packet to APRS-IS")
				err = i.Aprsis.Upload(msg)
				if err != nil {
					i.Logger.Error("Error uploading APRS packet: ", err)
					continue
				}

				if i.enableTx && packet.Type().NeedsAck() {
					ackMsg, err := packet.AckString()
					if err != nil {
						i.Logger.Error("Error creating APRS acknowledgement: ", err)
						continue
					}

					i.txChan <- ackMsg
				}
			}
		}
	}
}

func (i *IGate) startBeacon() error {
	if i.cfg.Beacon.Interval < (time.Duration(10) * time.Minute) {
		return fmt.Errorf("interval cannot be < 10m")
	}

	if i.cfg.Beacon.Call == "" {
		return fmt.Errorf("beacon call-sign not configured")
	}

	if !i.cfg.Beacon.Enabled {
		fmt.Println("beacon is disabled")
		return nil
	}

	i.Logger.Info("Starting beacon every ", i.cfg.Beacon.Interval)

	ticker := time.NewTicker(i.cfg.Beacon.Interval)

	go func() {
		for {
			select {
			case <-ticker.C:
				b := fmt.Sprintf("%s>BEACON:%s", i.cfg.Beacon.Call, i.cfg.Beacon.Comment)
				i.Logger.Info(b)
				i.Aprsis.Conn.PrintfLine(b)
			case <-i.Stop:
				ticker.Stop()
				return
			}
		}
	}()

	return nil
}

func (i *IGate) startTx() error {
	dataPort, err := config.DetectDataPort()
	if err != nil {
		return fmt.Errorf("Error detecting data port: %v", err)
	}

	c := &serial.Config{
		Name:        dataPort,
		Baud:        1200,
		ReadTimeout: time.Second * 5,
	}

	go func() {
		for {
			select {
			case <-i.Stop:
				return
			case msg := <-i.txChan:
				port, err := serial.OpenPort(c)
				if err != nil {
					i.Logger.Error("failed to open serial port: ", err)
				}

				fmtMsg := fmt.Sprintf("%v\r\n", msg)
				_, err = port.Write([]byte(fmtMsg))
				if err != nil {
					i.Logger.Error("failed to write to serial port: ", err)
				}

				i.Logger.Info("APRS message transmitted: ", msg)

				if err != nil {
					i.Logger.Error("Error transmitting APRS message: ", err)
				}

				port.Close()
			}
		}
	}()

	return nil
}
