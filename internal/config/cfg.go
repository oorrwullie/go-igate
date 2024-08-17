package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

type (
	Config struct {
		Sdr               Sdr         `yaml:"sdr"`
		Multimon          Multimon    `yaml:"multimon"`
		Transmitter       Transmitter `yaml:"transmitter"`
		IGate             IGate       `yaml:"igate"`
		DigipeaterEnabled bool        `yaml:"enable-digipeater"`
		CacheSize         int         `yaml:"cache-size"`
		StationCallsign   string      `yaml:"station-callsign"`
		SoundcardName     string      `yaml:"soundcard-name"`
	}

	IGate struct {
		Enabled bool   `yaml:"enabled"`
		Aprsis  AprsIs `yaml:"aprsis"`
		Beacon  Beacon `yaml:"beacon"`
	}

	Sdr struct {
		Enabled         bool `yaml:"enabled"`
		Path            string
		Frequency       string
		Device          string
		Gain            string
		PpmError        string `yaml:"ppm-error"`
		SquelchLevel    string `yaml:"squelch-level"`
		SampleRate      string `yaml:"sample-rate"`
		AdditionalFlags string `yaml:"additional-flags"`
	}

	Multimon struct {
		Path            string
		AdditionalFlags string `yaml:"additional-flags"`
	}

	Beacon struct {
		Enabled  bool
		Interval time.Duration
		Comment  string
	}

	AprsIs struct {
		Enabled  bool
		Server   string
		Passcode string
		Filter   string
	}

	Transmitter struct {
		Enabled bool `yaml:"enabled"`
	}
)

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
