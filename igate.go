package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

type (
	IGate struct {
		cfg          Config
		multimonChan chan []byte
		aprsisChan   chan string
		Stop         chan bool
		Aprsis       *AprsIs
		Logger       *Logger
	}
)

const minPacketSize = 35

func NewIGate() (*IGate, error) {
	cfg, err := GetConfig()
	if err != nil {
		return nil, fmt.Errorf("Error loading config: %v", err)
	}

	logger, err := NewLogger()
	if err != nil {
		return nil, err
	}

	aprsis, err := NewAprsIs(cfg, logger)
	if err != nil {
		return nil, err
	}

	ig := &IGate{
		cfg:          cfg,
		multimonChan: make(chan []byte),
		aprsisChan:   make(chan string),
		Stop:         make(chan bool),
		Aprsis:       aprsis,
		Logger:       logger,
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

	i.listenForMessages()

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
				i.Logger.Info("Multimon-ng output: ", msg)

				i.aprsisChan <- msg
			}

			if err := scanner.Err(); err != nil {
				i.Logger.Error("Error reading from multimon-ng: ", err)
			}

		}(outPipe)
	}()

	return nil
}

func (i *IGate) listenForMessages() {
	for {
		select {
		case <-i.Stop:
			return
		case msg := <-i.aprsisChan:
			if len(msg) < minPacketSize {
				i.Logger.Error("Packet too short: ", msg)
				continue
			}

			packet, err := i.Aprsis.ParsePacket(msg)
			if err != nil {
				i.Logger.Error(err, "Could not parse APRS packet")
				continue
			}

			i.Aprsis.Upload(packet)

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
				b := fmt.Sprintf("%s>BEACON: %s", i.cfg.Beacon.Call, i.cfg.Beacon.Comment)

				i.Logger.Info(b)
				i.Aprsis.conn.PrintfLine(b)
			case <-i.Stop:
				ticker.Stop()
				return
			}
		}
	}()

	return nil
}
