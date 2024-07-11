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
		aprsis  *AprsIs
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
		aprsis:  aprsis,
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

	err = i.listenForMessages()

	return err
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

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("Error starting rtl_fm: %v", err)
	}

	scanner := bufio.NewScanner(out)

	go func() {
		for scanner.Scan() {
			i.sdrChan <- scanner.Bytes()
		}
	}()

	return nil
}

func (i *IGate) startMultimon() error {
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
		return fmt.Errorf("Error opening multimon-ng stdin: %v", err)
	}

	multimonOut, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("Error opening multimon-ng stdout: %v", err)
	}

	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("Error starting multimon-ng: %v", err)
	}

	go func() {
		for data := range i.sdrChan {
			_, err := multimonIn.Write(data)
			if err != nil {
				fmt.Printf("Error writing to multimon-ng: %v", err)
			}
		}
		multimonIn.Close()
	}()

	scanner := bufio.NewScanner(multimonOut)
	for scanner.Scan() {
		i.msgChan <- scanner.Text()
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Error reading from multimon-ng: %v", err)
	}

	close(i.msgChan)

	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("Error waiting for multimon-ng: %v", err)
	}

	return nil
}

func (i *IGate) listenForMessages() error {
	for {
		select {
		case <-i.Stop:
			return nil
		case msg := <-i.msgChan:
			if len(msg) < minPacketSize {
				continue
			}

			packet, err := i.aprsis.ParsePacket(msg)
			if err != nil {
				i.Logger.Error(err, "Could not parse APRS packet")
				continue
			}

			i.aprsis.Upload(packet)

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

	fmt.Printf("Starting beacon every %s\n", i.cfg.Beacon.Interval)

	ticker := time.NewTicker(i.cfg.Beacon.Interval)

	go func() {
		for {
			select {
			case <-ticker.C:
				b := fmt.Sprintf("%s>BEACON: %s", i.cfg.Beacon.Call, i.cfg.Beacon.Comment)

				i.Logger.Info(b)
				i.aprsis.conn.PrintfLine(b)
			case <-i.Stop:
				ticker.Stop()
				return
			}
		}
	}()

	return nil
}
