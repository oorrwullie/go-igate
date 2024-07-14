package main

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Sdr struct {
		Path            string
		Frequency       string
		Device          string
		Gain            string
		PpmError        string `yaml:"ppm-error"`
		SquelchLevel    string `yaml:"squelch-level"`
		SampleRate      string `yaml:"sample-rate"`
		AdditionalFlags string `yaml:"additional-flags"`
	} `yaml:"sdr"`
	Multimon struct {
		Path            string
		AdditionalFlags string `yaml:"additional-flags"`
	} `yaml:"multimon"`
	Beacon struct {
		Enabled  bool
		Call     string `yaml:"call-sign"`
		Interval time.Duration
		Comment  string
	} `yaml:"beacon"`
	AprsIs struct {
		Id      string            `yaml:"id"`
		Options map[string]string `yaml:"options"`
	} `yaml:"aprsis"`
}

func GetConfig() (Config, error) {
	var cfg Config

	f, err := os.ReadFile("config.yml")
	if err != nil {
		return cfg, fmt.Errorf("Could not load config file: %v", err)
	}

	err = yaml.Unmarshal(f, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("Could not parse config file: %v", err)
	}

	return cfg, nil
}
