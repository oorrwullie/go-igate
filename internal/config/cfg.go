package config

import (
	"fmt"
	"os"
	"strings"
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
		SoundcardCapture    bool        `yaml:"soundcard-capture-enabled"`
	}

	IGate struct {
		Enabled       bool   `yaml:"enabled"`
		Aprsis        AprsIs `yaml:"aprsis"`
		Beacon        Beacon `yaml:"beacon"`
		ForwardSelfRF bool   `yaml:"forward-self-rf"`
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
		Enabled         bool
		Interval        time.Duration
		RFInterval      time.Duration `yaml:"rf-interval"`
		ISInterval      time.Duration `yaml:"is-interval"`
		DisableRF       bool          `yaml:"disable-rf"`
		DisableTCP      bool          `yaml:"disable-tcp"`
		DisableISBeacon bool          `yaml:"disable-is-beacon"`
		MaxRFAttempts   int           `yaml:"max-rf-attempts"`
		Comment         string
		RFPath          string             `yaml:"rf-path"`
		ISPath          string             `yaml:"is-path"`
		ExtraRF         []RFBeacon         `yaml:"additional-rf-beacons"`
		AprsFi          AprsFiVerification `yaml:"aprsfi-verification"`
	}

	AprsIs struct {
		Enabled  bool
		Server   string
		Passcode string
		Filter   string
	}

	Transmitter struct {
		Enabled  bool          `yaml:"enabled"`
		TxDelay  time.Duration `yaml:"tx-delay"`
		TxTail   time.Duration `yaml:"tx-tail"`
		UseAplay bool          `yaml:"use-aplay"`
	}

	Digipeater struct {
		AliasPatterns []string      `yaml:"alias-patterns"`
		WidePatterns  []string      `yaml:"wide-patterns"`
		SSnPrefixes   []string      `yaml:"ssn-prefixes"`
		DedupeWindow  time.Duration `yaml:"dedupe-window"`
	}

	RFBeacon struct {
		Path     string        `yaml:"path"`
		Interval time.Duration `yaml:"interval"`
	}

	AprsFiVerification struct {
		Enabled          bool          `yaml:"enabled"`
		APIKey           string        `yaml:"api-key"`
		Delay            time.Duration `yaml:"delay"`
		MaxAttempts      int           `yaml:"max-attempts"`
		Timeout          time.Duration `yaml:"timeout"`
		WatchdogInterval time.Duration `yaml:"watchdog-interval"`
		WatchdogMaxAge   time.Duration `yaml:"watchdog-max-age"`
	}
)

func GetConfig() (Config, error) {
	var cfg Config
	const defaultTxDelay = 300 * time.Millisecond

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
			`^[A-Z]{2}[1-7]-[1-7]$`,
		}
	}

	for idx, prefix := range cfg.Digipeater.SSnPrefixes {
		cfg.Digipeater.SSnPrefixes[idx] = strings.ToUpper(strings.TrimSpace(prefix))
	}

	if cfg.IGate.Beacon.RFPath == "" {
		cfg.IGate.Beacon.RFPath = "WIDE1-1"
	}

	if cfg.IGate.Beacon.ISPath == "" {
		cfg.IGate.Beacon.ISPath = "TCPIP*"
	}

	if cfg.IGate.Beacon.RFInterval <= 0 {
		cfg.IGate.Beacon.RFInterval = cfg.IGate.Beacon.Interval
	}

	if cfg.IGate.Beacon.ISInterval <= 0 {
		cfg.IGate.Beacon.ISInterval = cfg.IGate.Beacon.Interval
	}

	if cfg.IGate.Beacon.DisableRF {
		cfg.IGate.Beacon.RFInterval = 0
	}

	if cfg.IGate.Beacon.DisableTCP || cfg.IGate.Beacon.DisableISBeacon {
		cfg.IGate.Beacon.ISInterval = 0
	}

	for idx := range cfg.IGate.Beacon.ExtraRF {
		cfg.IGate.Beacon.ExtraRF[idx].Path = strings.TrimSpace(cfg.IGate.Beacon.ExtraRF[idx].Path)
	}

	if cfg.IGate.Beacon.AprsFi.Delay <= 0 {
		cfg.IGate.Beacon.AprsFi.Delay = 10 * time.Second
	}

	if cfg.IGate.Beacon.AprsFi.Timeout <= 0 {
		cfg.IGate.Beacon.AprsFi.Timeout = 10 * time.Second
	}

	if cfg.IGate.Beacon.AprsFi.MaxAttempts <= 0 {
		cfg.IGate.Beacon.AprsFi.MaxAttempts = 3
	}

	if cfg.IGate.Beacon.AprsFi.WatchdogInterval < 0 {
		cfg.IGate.Beacon.AprsFi.WatchdogInterval = 0
	}

	if cfg.IGate.Beacon.AprsFi.WatchdogInterval > 0 && cfg.IGate.Beacon.AprsFi.WatchdogMaxAge <= 0 {
		cfg.IGate.Beacon.AprsFi.WatchdogMaxAge = 2 * cfg.IGate.Beacon.AprsFi.WatchdogInterval
	}

	if cfg.IGate.Beacon.MaxRFAttempts <= 0 {
		cfg.IGate.Beacon.MaxRFAttempts = 3
	}

	if cfg.Transmitter.TxDelay <= 0 {
		cfg.Transmitter.TxDelay = defaultTxDelay
	}

	if cfg.Transmitter.TxTail <= 0 {
		cfg.Transmitter.TxTail = 100 * time.Millisecond
	}

	return cfg, nil
}
