package digipeater

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/pubsub"
	"github.com/oorrwullie/go-igate/internal/transmitter"
)

const minPacketSize = 35

type Digipeater struct {
	tx            *transmitter.Tx
	inputChan     <-chan string
	callsign      string
	callsignUpper string
	aliasRules    []*regexp.Regexp
	wideRules     []*regexp.Regexp
	dedupe        *deduper
	logger        *log.Logger
	stop          chan bool
}

func New(tx *transmitter.Tx, ps *pubsub.PubSub, callsign string, cfg config.Digipeater, logger *log.Logger) (*Digipeater, error) {
	if tx == nil {
		return nil, fmt.Errorf("digipeater requires a transmitter")
	}

	if ps == nil {
		return nil, fmt.Errorf("digipeater requires a pubsub")
	}

	if callsign == "" {
		return nil, fmt.Errorf("station callsign not configured")
	}

	alias, err := compilePatterns(cfg.AliasPatterns)
	if err != nil {
		return nil, fmt.Errorf("invalid alias pattern: %w", err)
	}

	wide, err := compilePatterns(cfg.WidePatterns)
	if err != nil {
		return nil, fmt.Errorf("invalid wide pattern: %w", err)
	}

	window := cfg.DedupeWindow
	if window <= 0 {
		window = 30 * time.Second
	}

	return &Digipeater{
		tx:            tx,
		inputChan:     ps.Subscribe(),
		callsign:      callsign,
		callsignUpper: strings.ToUpper(callsign),
		aliasRules:    alias,
		wideRules:     wide,
		dedupe:        newDeduper(window),
		logger:        logger,
		stop:          make(chan bool),
	}, nil
}

func (d *Digipeater) Run() error {
	d.logger.Info("Starting digipeater...")

	for {
		select {
		case <-d.stop:
			return nil
		case msg, ok := <-d.inputChan:
			if !ok {
				return nil
			}
			d.HandleMessage(msg)
		}
	}
}

func (d *Digipeater) Stop() {
	d.logger.Info("Stopping digipeater...")
	d.stop <- true
}

func (d *Digipeater) HandleMessage(msg string) {
	txMsg, ok := d.prepare(msg)
	if !ok {
		return
	}

	d.logger.Info("Digipeating packet: ", txMsg)
	go d.tx.Send(txMsg)
}

func (d *Digipeater) prepare(msg string) (string, bool) {
	if len(msg) < minPacketSize {
		d.logger.Debug("Packet too short to process: ", msg)
		return "", false
	}

	packet, err := aprs.ParsePacket(msg)
	if err != nil {
		d.logger.Error(err, "Failed to parse APRS packet: ", msg)
		return "", false
	}

	if strings.EqualFold(packet.Src, d.callsign) {
		return "", false
	}

	if len(packet.Path) == 0 {
		return "", false
	}

	firstIdx := firstUnusedPathIndex(packet.Path)
	if firstIdx == -1 {
		return "", false
	}

	key := dedupeKey(packet)
	if d.dedupe.Check(key) {
		d.logger.Debug("Suppressing duplicate packet: ", msg)
		return "", false
	}

	if !d.rewritePath(packet, firstIdx) {
		return "", false
	}

	d.dedupe.Remember(key)
	return formatPacket(packet), true
}

func (d *Digipeater) rewritePath(packet *aprs.Packet, idx int) bool {
	component := strings.TrimSpace(packet.Path[idx])
	if component == "" {
		return false
	}

	bare := strings.TrimSuffix(component, "*")

	if strings.EqualFold(bare, d.callsign) {
		packet.Path[idx] = d.callsignUpper + "*"
		return true
	}

	for _, re := range d.aliasRules {
		if re.MatchString(bare) {
			packet.Path[idx] = d.callsignUpper + "*"
			return true
		}
	}

	for _, re := range d.wideRules {
		if re.MatchString(bare) {
			base, remaining, ok := parseHop(bare)
			if !ok || remaining <= 0 {
				return false
			}

			if remaining == 1 {
				packet.Path[idx] = d.callsignUpper + "*"
				return true
			}

			packet.Path[idx] = fmt.Sprintf("%s-%d", base, remaining-1)
			packet.Path = insertBefore(packet.Path, idx, d.callsignUpper+"*")
			return true
		}
	}

	return false
}

func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	regexes := make([]*regexp.Regexp, 0, len(patterns))

	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}

		if !strings.HasPrefix(pattern, "(?i)") {
			pattern = "(?i)" + pattern
		}

		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		regexes = append(regexes, re)
	}

	return regexes, nil
}

func firstUnusedPathIndex(path []string) int {
	for idx, component := range path {
		component = strings.TrimSpace(component)
		if component == "" {
			continue
		}

		if !strings.Contains(component, "*") {
			return idx
		}
	}

	return -1
}

func parseHop(component string) (string, int, bool) {
	parts := strings.SplitN(component, "-", 2)
	if len(parts) != 2 {
		return "", 0, false
	}

	value, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, false
	}

	return parts[0], value, true
}

func insertBefore(path []string, idx int, value string) []string {
	if idx < 0 || idx > len(path) {
		return path
	}

	path = append(path[:idx], append([]string{value}, path[idx:]...)...)
	return path
}

func formatPacket(packet *aprs.Packet) string {
	var builder strings.Builder
	builder.WriteString(packet.Src)
	builder.WriteString(">")
	builder.WriteString(packet.Dst)

	if len(packet.Path) > 0 {
		builder.WriteString(",")
		builder.WriteString(strings.Join(packet.Path, ","))
	}

	builder.WriteString(":")
	builder.WriteString(packet.Payload)

	return builder.String()
}

func dedupeKey(packet *aprs.Packet) string {
	return strings.ToUpper(packet.Src) + ">" + strings.ToUpper(packet.Dst) + ":" + packet.Payload
}

type deduper struct {
	window  time.Duration
	mu      sync.Mutex
	entries map[string]time.Time
}

func newDeduper(window time.Duration) *deduper {
	return &deduper{
		window:  window,
		entries: make(map[string]time.Time),
	}
}

func (d *deduper) Check(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	d.evictExpired(now)

	if ts, ok := d.entries[key]; ok && now.Sub(ts) <= d.window {
		return true
	}

	return false
}

func (d *deduper) Remember(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	d.evictExpired(now)
	d.entries[key] = now
}

func (d *deduper) evictExpired(now time.Time) {
	for key, ts := range d.entries {
		if now.Sub(ts) > d.window {
			delete(d.entries, key)
		}
	}
}
