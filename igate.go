package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type (
	IGate struct {
		cfg     Config
		sdrChan chan []byte
		msgChan chan string
		Stop    chan bool
		Aprsis  *AprsIs
		Logger  *Logger
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
		cfg:     cfg,
		sdrChan: make(chan []byte),
		msgChan: make(chan string),
		Stop:    make(chan bool),
		Aprsis:  aprsis,
		Logger:  logger,
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

	<-i.Stop

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

	scanner := bufio.NewScanner(out)

	go func() {
		for scanner.Scan() {
			i.sdrChan <- scanner.Bytes()
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

		multimonIn, err := cmd.StdinPipe()
		if err != nil {
			i.Logger.Error("Error opening multimon-ng stdin:", err)
			return
		}

		multimonOut, err := cmd.StdoutPipe()
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

		go func() {
			for data := range i.sdrChan {
				_, err := multimonIn.Write(data)
				if err != nil {
					i.Logger.Error("Error writing to multimon-ng: ", err)
				}
			}
		}()

		scanner := bufio.NewScanner(multimonOut)
		for {
			if scanner.Scan() {

				line := scanner.Text()
				i.Logger.Info("Got here. Processing a line: ", line)

				if len(line) < minPacketSize {
					i.Logger.Error("Packet too short: ", line)
					continue
				}

				packet, err := i.Aprsis.ParsePacket(line)
				if err != nil {
					i.Logger.Error(err, "Could not parse APRS packet")
					continue
				}

				i.Aprsis.Upload(packet)
			} else {
				i.Logger.Error("multimon-ng pooped the bed :-(")
				break

			}
		}

		if err := scanner.Err(); err != nil {
			i.Logger.Error("Error reading from multimon-ng: ", err)
		}

		if err := cmd.Wait(); err != nil {
			i.Logger.Error("Error waiting for multimon-ng: ", err)
		}
	}()

	return nil
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
