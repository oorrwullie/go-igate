package config

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

type (
	Config struct {
		Sdr                 Sdr           `yaml:"sdr"`
		OpenWebRXMQTT       OpenWebRXMQTT `yaml:"openwebrx-mqtt"`
		MapFlights          MapFlights    `yaml:"map-flights"`
		Multimon            Multimon      `yaml:"multimon"`
		Transmitter         Transmitter   `yaml:"transmitter"`
		IGate               IGate         `yaml:"igate"`
		DigipeaterEnabled   bool          `yaml:"enable-digipeater"`
		Digipeater          Digipeater    `yaml:"digipeater"`
		CacheSize           int           `yaml:"cache-size"`
		StationCallsign     string        `yaml:"station-callsign"`
		SoundcardInputName  string        `yaml:"soundcard-input-name"`
		SoundcardOutputName string        `yaml:"soundcard-output-name"`
		SoundcardCapture    bool          `yaml:"soundcard-capture-enabled"`
	}

	IGate struct {
		Enabled             bool          `yaml:"enabled"`
		Aprsis              AprsIs        `yaml:"aprsis"`
		Beacon              Beacon        `yaml:"beacon"`
		ForwardSelfRF       bool          `yaml:"forward-self-rf"`
		GateDigipeatedPath  bool          `yaml:"gate-digipeated-path"`
		MessageGating       bool          `yaml:"message-gating"`
		MessageRFPath       string        `yaml:"message-rf-path"`
		MessageRFInterval   time.Duration `yaml:"message-rf-interval"`
		MessageDedupWindow  time.Duration `yaml:"message-dedup-window"`
		MessageGateDelay    time.Duration `yaml:"message-gate-delay"`
		LocalStationTimeout time.Duration `yaml:"local-station-timeout"`
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

	OpenWebRXMQTT struct {
		Enabled        bool               `yaml:"enabled"`
		Broker         string             `yaml:"broker"`
		Topic          string             `yaml:"topic"`
		Username       string             `yaml:"username"`
		Password       string             `yaml:"password"`
		EmbeddedBroker EmbeddedMQTTBroker `yaml:"embedded-broker"`
	}

	EmbeddedMQTTBroker struct {
		Enabled bool   `yaml:"enabled"`
		Listen  string `yaml:"listen-address"`
	}

	MapFlights struct {
		Enabled bool   `yaml:"enabled"`
		Broker  string `yaml:"broker"`
		Station string `yaml:"station"`
		Key     string `yaml:"key"`
		Topic   string `yaml:"topic"`
	}

	Multimon struct {
		Path               string
		AdditionalFlags    string `yaml:"additional-flags"`
		ReceiveDiagnostics bool   `yaml:"receive-diagnostics"`
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
		Enabled        bool
		Server         string
		Passcode       string
		Filter         string
		MuteInboundLog bool `yaml:"mute-inbound-log"`
	}

	Transmitter struct {
		Enabled    bool          `yaml:"enabled"`
		TxDelay    time.Duration `yaml:"tx-delay"`
		TxTail     time.Duration `yaml:"tx-tail"`
		AudioLevel float64       `yaml:"audio-level"`
		UseAplay   bool          `yaml:"use-aplay"`
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
	const defaultTxDelay = time.Second

	f, err := os.ReadFile("config.yml")
	if err != nil {
		return cfg, fmt.Errorf("Could not load config file: %v", err)
	}

	err = yaml.Unmarshal(f, &cfg)
	if err != nil {
		return cfg, fmt.Errorf("Could not parse config file: %v", err)
	}
	if cfg.OpenWebRXMQTT.EmbeddedBroker.Enabled {
		address := strings.TrimSpace(cfg.OpenWebRXMQTT.EmbeddedBroker.Listen)
		if address == "" {
			address = "127.0.0.1:1884"
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return cfg, fmt.Errorf("invalid openwebrx-mqtt.embedded-broker.listen-address %q: %w", address, err)
		}
		if host != "localhost" {
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				return cfg, fmt.Errorf("openwebrx-mqtt.embedded-broker.listen-address must bind to loopback; embedded broker does not provide authentication or TLS")
			}
		}
	}
	if cfg.MapFlights.Enabled {
		if !cfg.OpenWebRXMQTT.Enabled {
			return cfg, fmt.Errorf("map-flights requires openwebrx-mqtt.enabled")
		}
		if strings.TrimSpace(cfg.MapFlights.Station) == "" || strings.TrimSpace(cfg.MapFlights.Key) == "" {
			return cfg, fmt.Errorf("map-flights.station and map-flights.key are required when map-flights.enabled is true")
		}
		if topic := strings.TrimSpace(cfg.MapFlights.Topic); strings.ContainsAny(topic, "+#") {
			return cfg, fmt.Errorf("map-flights.topic must be one concrete topic, not an MQTT wildcard")
		}
		broker := strings.TrimSpace(cfg.MapFlights.Broker)
		if broker != "" && !strings.HasPrefix(strings.ToLower(broker), "ssl://") {
			return cfg, fmt.Errorf("map-flights.broker must use ssl:// because the station key is sent as the MQTT password")
		}
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

	if cfg.Transmitter.AudioLevel == 0 {
		cfg.Transmitter.AudioLevel = 0.9
	}
	if cfg.Transmitter.AudioLevel < 0 || cfg.Transmitter.AudioLevel > 1 {
		return cfg, fmt.Errorf("transmitter audio-level must be between 0 and 1")
	}

	if cfg.IGate.LocalStationTimeout <= 0 {
		cfg.IGate.LocalStationTimeout = time.Hour
	}

	if cfg.IGate.MessageRFInterval <= 0 {
		cfg.IGate.MessageRFInterval = 5 * time.Second
	}

	if cfg.IGate.MessageDedupWindow <= 0 {
		cfg.IGate.MessageDedupWindow = time.Minute
	}

	if cfg.IGate.MessageGateDelay <= 0 {
		cfg.IGate.MessageGateDelay = 2 * time.Second
	}

	return cfg, nil
}
