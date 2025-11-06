package igate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/log"
	"github.com/oorrwullie/go-igate/internal/pubsub"
	"github.com/oorrwullie/go-igate/internal/transmitter"
	"golang.org/x/sync/errgroup"
)

type (
	IGate struct {
		cfg             config.IGate
		callSign        string
		inputChan       <-chan string
		enableTx        bool
		tx              *transmitter.Tx
		logger          *log.Logger
		Aprsis          *aprs.AprsIs
		aprsisUpload    func(string) error
		httpClient      *http.Client
		verifyAprsFi    bool
		aprsFiKey       string
		aprsFiDelay     time.Duration
		aprsFiTries     int
		aprsFiBaseURL   string
		disableISBeacon bool
		maxRfAttempts   int
		forwardChan     chan *aprs.Packet
		stop            chan struct{}
		stopOnce        sync.Once
		rfBeaconMu      sync.Mutex
		beaconMu        sync.Mutex
		isBeaconMu      sync.Mutex
		beaconWaits     map[string]*beaconWait
		lastRxMu        sync.Mutex
		lastRx          time.Time
	}
)

type beaconWait struct {
	frame   string
	payload string
	started time.Time
	result  chan bool
}

type beaconOutcome int

const (
	beaconOutcomeSuccess beaconOutcome = iota
	beaconOutcomeCollision
	beaconOutcomeTimeout
	beaconOutcomeStopped
)

const minPacketSize = 35
const forwarderQueueSize = 32
const defaultAprsFiBaseURL = "https://api.aprs.fi"
const aprsFiGracePeriod = 10 * time.Second

type aprsFiResponse struct {
	Result      string        `json:"result"`
	Description string        `json:"description"`
	Entries     []aprsFiEntry `json:"entries"`
}

type aprsFiEntry struct {
	Lasttime string `json:"lasttime"`
}

var (
	beaconChannelQuiet    = 3 * time.Second
	beaconRetryInterval   = 5 * time.Second
	beaconCollisionWindow = 5 * time.Second
	beaconWarmup          = 2 * time.Second
)

func New(cfg config.IGate, ps *pubsub.PubSub, enableTx bool, tx *transmitter.Tx, callSign string, logger *log.Logger) (*IGate, error) {
	aprsis, err := aprs.New(cfg.Aprsis, callSign, cfg.Beacon.Comment, logger)
	if err != nil {
		return nil, fmt.Errorf("Error creating APRS client: %v", err)
	}

	inputChan := ps.Subscribe()

	var (
		httpClient   *http.Client
		verifyAprsFi bool
		apiKey       string
	)

	if cfg.Beacon.AprsFi.Enabled {
		apiKey = strings.TrimSpace(cfg.Beacon.AprsFi.APIKey)
		if apiKey == "" {
			logger.Warn("APRS-IS verification enabled but api-key missing; disabling verification")
		} else {
			timeout := cfg.Beacon.AprsFi.Timeout
			if timeout <= 0 {
				timeout = 10 * time.Second
			}
			httpClient = &http.Client{
				Timeout: timeout,
			}
			verifyAprsFi = true
		}
	}

	disableISBeacon := cfg.Beacon.DisableTCP || cfg.Beacon.DisableISBeacon

	ig := &IGate{
		cfg:             cfg,
		callSign:        callSign,
		inputChan:       inputChan,
		enableTx:        enableTx,
		tx:              tx,
		logger:          logger,
		Aprsis:          aprsis,
		aprsisUpload:    aprsis.Upload,
		httpClient:      httpClient,
		verifyAprsFi:    verifyAprsFi,
		aprsFiKey:       apiKey,
		aprsFiDelay:     cfg.Beacon.AprsFi.Delay,
		aprsFiTries:     cfg.Beacon.AprsFi.MaxAttempts,
		aprsFiBaseURL:   defaultAprsFiBaseURL,
		disableISBeacon: disableISBeacon,
		maxRfAttempts:   cfg.Beacon.MaxRFAttempts,
		forwardChan:     make(chan *aprs.Packet, forwarderQueueSize),
		stop:            make(chan struct{}),
		lastRx:          time.Now(),
		beaconWaits:     make(map[string]*beaconWait),
	}

	return ig, nil
}

func (i *IGate) Run() error {
	if i.cfg.Beacon.Enabled {
		err := i.startBeacon()
		if err != nil {
			return fmt.Errorf("Error starting beacon: %v", err)
		}
	}

	var g errgroup.Group

	g.Go(func() error {
		return i.forwardPackets()
	})

	g.Go(func() error {
		return i.listenForMessages()
	})

	return g.Wait()
}

func (i *IGate) Stop() {
	i.stopOnce.Do(func() {
		close(i.stop)
		i.cancelPendingBeacon()
	})
}

func (i *IGate) listenForMessages() error {
	defer close(i.forwardChan)

	for {
		select {
		case <-i.stop:
			return nil
		case msg, ok := <-i.inputChan:
			if !ok {
				return nil
			}
			i.markRx()

			if len(msg) < minPacketSize {
				i.logger.Error("Packet too short: ", msg)
				continue
			}

			packet, err := aprs.ParsePacket(msg)
			if err != nil {
				i.logger.Error(err, "Could not parse APRS packet: ", msg)
				continue
			}

			selfPacket := strings.EqualFold(packet.Src, i.callSign)
			hasPath := len(packet.Path) > 0 && strings.TrimSpace(strings.Join(packet.Path, "")) != ""
			if selfPacket && !i.cfg.ForwardSelfRF {
				i.logger.Debug("Skipping APRS-IS forwarding of self-originated packet: ", msg)
				i.observeBeacon(packet)
				continue
			}
			if selfPacket && i.cfg.ForwardSelfRF && !hasPath {
				i.logger.Debug("Skipping APRS-IS forwarding of pathless self packet: ", msg)
				i.observeBeacon(packet)
				continue
			}

			shouldForward := !packet.IsAckMessage() && packet.Type().ForwardToAprsIs()
			if shouldForward {
				if selfPacket {
					i.logger.Debug("Forwarding self-originated RF packet to APRS-IS: ", msg)
				}
				select {
				case <-i.stop:
					return nil
				case i.forwardChan <- packet:
				}
			}

			i.observeBeacon(packet)
		}
	}
}

func (i *IGate) forwardPackets() error {
	for {
		select {
		case <-i.stop:
			return nil
		case packet, ok := <-i.forwardChan:
			if !ok {
				return nil
			}

			if packet == nil {
				continue
			}

			uploadFrame := formatForAprsIs(packet, i.callSign)
			fmt.Printf("uploading APRS-IS packet: %v\n", uploadFrame)
			if i.aprsisUpload != nil {
				if err := i.aprsisUpload(uploadFrame); err != nil {
					i.logger.Error("Error uploading APRS packet: ", err)
				}
			} else {
				i.logger.Debug("Skipping APRS-IS upload; APRS client not configured")
			}
		}
	}
}

func (i *IGate) startBeacon() error {
	const minInterval = 10 * time.Minute

	rfInterval := i.cfg.Beacon.RFInterval
	isInterval := i.cfg.Beacon.ISInterval
	extraRF := i.cfg.Beacon.ExtraRF
	disableISBeacon := i.disableISBeacon

	if !i.cfg.Beacon.Enabled {
		fmt.Println("beacon is disabled")
		return nil
	}

	if !i.cfg.Beacon.DisableRF && rfInterval <= 0 && len(extraRF) == 0 && i.cfg.Beacon.Interval > 0 {
		rfInterval = i.cfg.Beacon.Interval
	}

	if !disableISBeacon && isInterval <= 0 && i.cfg.Beacon.Interval > 0 {
		isInterval = i.cfg.Beacon.Interval
	}

	if i.cfg.Beacon.DisableRF {
		rfInterval = 0
	}

	if disableISBeacon {
		isInterval = 0
	}

	if rfInterval > 0 && rfInterval < minInterval {
		return fmt.Errorf("rf-interval cannot be < 10m")
	}

	if isInterval > 0 && isInterval < minInterval {
		return fmt.Errorf("is-interval cannot be < 10m")
	}

	type rfSchedule struct {
		path     string
		interval time.Duration
	}

	var rfSchedules []rfSchedule

	if !i.cfg.Beacon.DisableRF {
		if rfInterval > 0 {
			rfSchedules = append(rfSchedules, rfSchedule{
				path:     strings.TrimSpace(i.cfg.Beacon.RFPath),
				interval: rfInterval,
			})
		}

		for _, schedule := range extraRF {
			if schedule.Interval <= 0 {
				return fmt.Errorf("additional-rf-beacons interval must be > 0")
			}

			if schedule.Interval < minInterval {
				return fmt.Errorf("additional-rf-beacons interval cannot be < 10m")
			}

			rfSchedules = append(rfSchedules, rfSchedule{
				path:     schedule.Path,
				interval: schedule.Interval,
			})
		}
	}

	if len(rfSchedules) == 0 && isInterval <= 0 {
		if i.cfg.Beacon.DisableRF && disableISBeacon {
			fmt.Println("beacon destinations disabled")
			return nil
		}

		return fmt.Errorf("beacon interval not configured")
	}

	if i.callSign == "" {
		return fmt.Errorf("beacon call-sign not configured")
	}

	switch {
	case rfInterval > 0 && isInterval > 0:
		i.logger.Info("Starting beacon schedule RF every ", rfInterval, " and APRS-IS every ", isInterval)
	case rfInterval > 0:
		i.logger.Info("Starting beacon schedule RF every ", rfInterval)
	case isInterval > 0:
		i.logger.Info("Starting beacon schedule APRS-IS every ", isInterval)
	}

	for _, schedule := range rfSchedules {
		path := schedule.path
		if path == "" {
			path = "(direct)"
		}
		i.logger.Info("RF beacon path ", path, " every ", schedule.interval)
	}

	chainIsToRf := !disableISBeacon && len(rfSchedules) > 0
	if chainIsToRf {
		i.logger.Info("Sequencing APRS-IS beacon immediately before each RF beacon")
	}

	var isTicker *time.Ticker
	var isChan <-chan time.Time
	if isInterval > 0 && !chainIsToRf {
		isTicker = time.NewTicker(isInterval)
		isChan = isTicker.C
	}

	sendAprsIs := func(reason string) {
		if disableISBeacon || i.aprsisUpload == nil {
			return
		}

		i.isBeaconMu.Lock()
		defer i.isBeaconMu.Unlock()

		isFrame := buildBeaconFrame(i.callSign, i.cfg.Beacon.ISPath, i.cfg.Beacon.Comment)
		if reason != "" {
			i.logger.Info("Beacon -> APRS-IS (", reason, "): ", isFrame)
		} else {
			i.logger.Info("Beacon -> APRS-IS: ", isFrame)
		}

		if err := i.aprsisUpload(isFrame); err != nil {
			i.logger.Error("Error uploading APRS beacon: ", err)
		}
	}

	sendRF := func(path, reason string) {
		if i.cfg.Beacon.DisableRF || !i.enableTx || i.tx == nil {
			return
		}

		if chainIsToRf {
			sendAprsIs(reason)
		}

		rfFrame := buildBeaconFrame(i.callSign, path, i.cfg.Beacon.Comment)
		i.logger.Info("Beacon -> RF (", reason, "): ", rfFrame)
		go i.sendBeaconRf(rfFrame, i.cfg.Beacon.Comment, true)
	}

	// Send initial beacon to each configured destination
	if isInterval > 0 && !chainIsToRf {
		sendAprsIs("initial")
	}

	for _, schedule := range rfSchedules {
		sendRF(schedule.path, "initial")
	}

	if isTicker != nil {
		go func() {
			defer isTicker.Stop()

			for {
				select {
				case <-i.stop:
					return
				case <-isChan:
					sendAprsIs("scheduled")
				}
			}
		}()
	}

	for _, schedule := range rfSchedules {
		s := schedule
		go func() {
			ticker := time.NewTicker(s.interval)
			defer ticker.Stop()

			for {
				select {
				case <-i.stop:
					return
				case <-ticker.C:
					sendRF(s.path, "scheduled")
				}
			}
		}()
	}

	return nil
}

func buildBeaconFrame(callSign, path, comment string) string {
	path = strings.TrimSpace(path)

	var builder strings.Builder
	builder.WriteString(callSign)
	builder.WriteString(">APRS")

	if path != "" {
		builder.WriteString(",")
		builder.WriteString(path)
	}

	builder.WriteString(":")
	builder.WriteString(comment)

	return builder.String()
}

func formatForAprsIs(packet *aprs.Packet, callSign string) string {
	var builder strings.Builder

	callSign = strings.ToUpper(strings.TrimSpace(callSign))

	builder.WriteString(packet.Src)
	builder.WriteString(">")
	builder.WriteString(packet.Dst)

	path := append([]string{}, packet.Path...)
	if !containsAprsIsHop(path) && callSign != "" {
		path = append(path, "TCPIP*", "qAR", callSign)
	}

	if len(path) > 0 {
		builder.WriteString(",")
		builder.WriteString(strings.Join(path, ","))
	}

	builder.WriteString(":")
	builder.WriteString(packet.Payload)

	return builder.String()
}

func containsAprsIsHop(path []string) bool {
	for _, component := range path {
		c := strings.ToUpper(strings.TrimSpace(component))
		if strings.HasPrefix(c, "TCPIP") || strings.HasPrefix(c, "TCPXX") || strings.HasPrefix(c, "Q") {
			return true
		}
	}

	return false
}

func (i *IGate) markRx() {
	i.lastRxMu.Lock()
	i.lastRx = time.Now()
	i.lastRxMu.Unlock()
}

func (i *IGate) channelQuietFor(duration time.Duration) bool {
	i.lastRxMu.Lock()
	defer i.lastRxMu.Unlock()

	if i.lastRx.IsZero() {
		return true
	}

	return time.Since(i.lastRx) >= duration
}

func (i *IGate) sendBeaconRf(frame, payload string, allowRetry bool) {
	i.rfBeaconMu.Lock()
	defer i.rfBeaconMu.Unlock()

	attempts := 0

	for {
		if i.maxRfAttempts > 0 && attempts >= i.maxRfAttempts {
			// TODO - If you want to force a packet to the internet to keep your station on the map,
			//        uncomment the next two lines. I found in testing it always does it this way and I
			//        didn't like it. I commented it out.
			//i.logger.Warn("RF beacon failed after ", attempts, " attempts; forcing APRS-IS beacon")
			//i.forceAprsIsBeacon()
			return
		}
		attempts++

		if !i.waitForQuiet(beaconChannelQuiet) {
			return
		}

		if !i.sleepWithStop(beaconWarmup) {
			return
		}

		sendStart := time.Now()
		key, outcomeCh := i.expectBeacon(frame, payload, sendStart)

		i.logger.Info("Beacon -> RF: ", frame)
		i.tx.Send(frame)

		outcome := i.waitForBeaconOutcome(outcomeCh)
		i.clearBeaconExpectation(key, outcomeCh)

		if outcome == beaconOutcomeStopped {
			return
		}

		if outcome == beaconOutcomeSuccess {
			i.logger.Debug("Beacon RF transmission confirmed without collision")
			i.scheduleAprsFiVerification(frame, payload, sendStart, allowRetry)
			return
		}

		if outcome == beaconOutcomeTimeout {
			i.logger.Debug("Beacon RF transmission not observed; assuming success")
			i.scheduleAprsFiVerification(frame, payload, sendStart, allowRetry)
			return
		}

		i.logger.Warn("Beacon collision detected; backing off before retry")
		if !i.sleepWithStop(beaconRetryInterval) {
			return
		}
	}
}

func (i *IGate) waitForQuiet(duration time.Duration) bool {
	for {
		if i.channelQuietFor(duration) {
			return true
		}

		if !i.sleepWithStop(beaconRetryInterval) {
			return false
		}
	}
}

func (i *IGate) sleepWithStop(d time.Duration) bool {
	if d <= 0 {
		return true
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-i.stop:
		return false
	case <-timer.C:
		return true
	}
}

func (i *IGate) expectBeacon(frame, payload string, started time.Time) (string, <-chan bool) {
	result := make(chan bool, 1)

	key := strings.TrimSpace(frame)
	payload = strings.TrimSpace(payload)

	i.beaconMu.Lock()
	if i.beaconWaits == nil {
		i.beaconWaits = make(map[string]*beaconWait)
	}
	i.beaconWaits[key] = &beaconWait{
		frame:   key,
		payload: payload,
		started: started,
		result:  result,
	}
	i.beaconMu.Unlock()

	return key, result
}

func (i *IGate) waitForBeaconOutcome(ch <-chan bool) beaconOutcome {
	timer := time.NewTimer(beaconCollisionWindow)
	defer timer.Stop()

	select {
	case <-i.stop:
		return beaconOutcomeStopped
	case res := <-ch:
		if res {
			return beaconOutcomeSuccess
		}
		return beaconOutcomeCollision
	case <-timer.C:
		return beaconOutcomeTimeout
	}
}

func (i *IGate) clearBeaconExpectation(key string, ch <-chan bool) {
	i.beaconMu.Lock()
	defer i.beaconMu.Unlock()

	if i.beaconWaits == nil {
		return
	}

	if wait, ok := i.beaconWaits[key]; ok && wait.result == ch {
		delete(i.beaconWaits, key)
	}
}

func (i *IGate) observeBeacon(packet *aprs.Packet) {
	i.beaconMu.Lock()
	defer i.beaconMu.Unlock()

	if len(i.beaconWaits) == 0 {
		return
	}

	payload := strings.TrimSpace(packet.Payload)

	path := strings.Join(packet.Path, ",")
	frame := buildBeaconFrame(packet.Src, path, payload)

	if wait, ok := i.beaconWaits[frame]; ok && wait.payload == payload {
		select {
		case wait.result <- true:
		default:
		}
		delete(i.beaconWaits, frame)
		return
	}

	if i.isSelfPacket(packet) && payload != "" {
		for key, wait := range i.beaconWaits {
			if wait.payload == payload {
				select {
				case wait.result <- true:
				default:
				}
				delete(i.beaconWaits, key)
				return
			}
		}
	}

	now := time.Now()
	for key, wait := range i.beaconWaits {
		if now.Sub(wait.started) <= beaconCollisionWindow {
			select {
			case wait.result <- false:
			default:
			}
			delete(i.beaconWaits, key)
		}
	}
}

func (i *IGate) isSelfPacket(packet *aprs.Packet) bool {
	if packet == nil {
		return false
	}

	if strings.EqualFold(packet.Src, i.callSign) {
		return true
	}

	self := strings.ToUpper(strings.TrimSpace(i.callSign))
	if self == "" {
		return false
	}

	for _, hop := range packet.Path {
		hop = strings.TrimSpace(hop)
		if hop == "" {
			continue
		}
		hop = strings.TrimSuffix(strings.ToUpper(hop), "*")
		if hop == self {
			return true
		}
	}

	return false
}

func (i *IGate) cancelPendingBeacon() {
	i.beaconMu.Lock()
	defer i.beaconMu.Unlock()

	if len(i.beaconWaits) == 0 {
		return
	}

	for key, wait := range i.beaconWaits {
		select {
		case wait.result <- false:
		default:
		}
		delete(i.beaconWaits, key)
	}
}

func (i *IGate) scheduleAprsFiVerification(frame, payload string, sent time.Time, allowRetry bool) {
	if !i.verifyAprsFi || !i.disableISBeacon {
		return
	}
	if i.httpClient == nil || strings.TrimSpace(i.aprsFiKey) == "" {
		return
	}
	if sent.IsZero() {
		return
	}

	go i.runAprsFiVerification(frame, payload, sent, allowRetry)
}

func (i *IGate) runAprsFiVerification(frame, payload string, sent time.Time, allowRetry bool) {
	delay := i.aprsFiDelay
	if delay <= 0 {
		delay = 30 * time.Second
	}

	attempts := i.aprsFiTries
	if attempts <= 0 {
		attempts = 3
	}

	wait := delay
	for attempt := 1; attempt <= attempts; attempt++ {
		if !i.sleepWithStop(wait) {
			return
		}

		ok, err := i.queryAprsFi(sent)
		if err != nil {
			i.logger.Warn("APRS.fi verification attempt ", attempt, " failed: ", err)
		}
		if ok {
			i.logger.Debug("APRS.fi verification confirmed beacon on attempt ", attempt)
			return
		}

		wait = aprsFiGracePeriod
	}

	if allowRetry {
		i.logger.Warn("APRS.fi did not confirm beacon; retrying RF beacon")
		go i.sendBeaconRf(frame, payload, false)
		return
	}

	i.logger.Warn("APRS.fi did not confirm beacon after RF retry; forcing APRS-IS upload")
	i.forceAprsIsBeacon()
}

func (i *IGate) queryAprsFi(sent time.Time) (bool, error) {
	if i.httpClient == nil {
		return false, fmt.Errorf("aprs.fi HTTP client not configured")
	}
	if strings.TrimSpace(i.aprsFiKey) == "" {
		return false, fmt.Errorf("aprs.fi api-key missing")
	}
	if sent.IsZero() {
		return false, fmt.Errorf("sent time not provided")
	}

	base := strings.TrimSpace(i.aprsFiBaseURL)
	if base == "" {
		base = defaultAprsFiBaseURL
	}
	base = strings.TrimRight(base, "/")

	params := url.Values{}
	params.Set("name", strings.ToUpper(strings.TrimSpace(i.callSign)))
	params.Set("what", "loc")
	params.Set("format", "json")
	params.Set("apikey", i.aprsFiKey)

	endpoint := fmt.Sprintf("%s/api/get?%s", base, params.Encode())

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return false, fmt.Errorf("aprs.fi status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	var result aprsFiResponse
	if err := json.Unmarshal(payload, &result); err != nil {
		return false, err
	}
	if !strings.EqualFold(result.Result, "ok") {
		if result.Description != "" {
			return false, fmt.Errorf("aprs.fi error: %s", result.Description)
		}
		return false, fmt.Errorf("aprs.fi returned result %q", result.Result)
	}

	var latest time.Time
	for _, entry := range result.Entries {
		if entry.Lasttime == "" {
			continue
		}
		ts, err := strconv.ParseInt(entry.Lasttime, 10, 64)
		if err != nil {
			continue
		}
		t := time.Unix(ts, 0)
		if t.After(latest) {
			latest = t
		}
	}

	if latest.IsZero() {
		return false, nil
	}

	threshold := sent.Add(-aprsFiGracePeriod)
	if !latest.Before(threshold) {
		return true, nil
	}

	return false, nil
}

func (i *IGate) forceAprsIsBeacon() {
	if i.aprsisUpload == nil {
		i.logger.Warn("Cannot force APRS-IS beacon; upload function not configured")
		return
	}

	frame := buildBeaconFrame(i.callSign, i.cfg.Beacon.ISPath, i.cfg.Beacon.Comment)

	i.isBeaconMu.Lock()
	defer i.isBeaconMu.Unlock()

	if err := i.aprsisUpload(frame); err != nil {
		i.logger.Error("Error forcing APRS-IS beacon: ", err)
		return
	}

	i.logger.Info("Forced APRS-IS beacon upload: ", frame)
}
