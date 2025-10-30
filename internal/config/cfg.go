package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

type (
	Config struct {
		Sdr                 Sdr         `yaml:"sdr"`
		Multimon            Multimon    `yaml:"multimon"`
		Transmitter         Transmitter `yaml:"transmitter"`
		IGate               IGate       `yaml:"igate"`
		DigipeaterEnabled   bool        `yaml:"enable-digipeater"`
		Digipeater          Digipeater  `yaml:"digipeater"`
		CacheSize           int         `yaml:"cache-size"`
		StationCallsign     string      `yaml:"station-callsign"`
		SoundcardInputName  string      `yaml:"soundcard-input-name"`
		SoundcardOutputName string      `yaml:"soundcard-output-name"`
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

	Digipeater struct {
		AliasPatterns []string      `yaml:"alias-patterns"`
		WidePatterns  []string      `yaml:"wide-patterns"`
		DedupeWindow  time.Duration `yaml:"dedupe-window"`
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

	if cfg.Digipeater.DedupeWindow == 0 {
		cfg.Digipeater.DedupeWindow = 30 * time.Second
	}

	if len(cfg.Digipeater.AliasPatterns) == 0 {
		cfg.Digipeater.AliasPatterns = []string{`^WIDE1-1$`}
	}

	if len(cfg.Digipeater.WidePatterns) == 0 {
		cfg.Digipeater.WidePatterns = []string{
			`^WIDE[1-7]-[1-7]$`,
			`^TRACE[1-7]-[1-7]$`,
			`^HOP[1-7]-[1-7]$`,
		}
	}

	return cfg, nil
}
