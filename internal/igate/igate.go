package igate

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oorrwullie/sparky/internal/aprs"
	"github.com/oorrwullie/sparky/internal/config"
	"github.com/oorrwullie/sparky/internal/digipeater"
	"github.com/oorrwullie/sparky/internal/log"
	"github.com/oorrwullie/sparky/internal/pubsub"
	"github.com/oorrwullie/sparky/internal/transmitter"
	"golang.org/x/sync/errgroup"
)

type (
	IGate struct {
		cfg                    config.IGate
		callSign               string
		inputChan              <-chan string
		enableTx               bool
		tx                     *transmitter.Tx
		logger                 *log.Logger
		Aprsis                 *aprs.AprsIs
		aprsisUpload           func(string) error
		forwardWrites atomic.Uint64
		forwardErrors atomic.Uint64
		httpClient             *http.Client
		verifyAprsFi           bool
		aprsFiKey              string
		aprsFiDelay            time.Duration
		aprsFiTries            int
		aprsFiBaseURL          string
		aprsFiWatchdogInterval time.Duration
		aprsFiWatchdogMaxAge   time.Duration
		disableISBeacon        bool
		maxRfAttempts          int
		geoFilter              *geoFilter
		forwardChan            chan *aprs.Packet
		stop                   chan struct{}
		stopOnce               sync.Once
		asyncMu                 sync.Mutex
		asyncWorkers            sync.WaitGroup
		verificationSlots       chan struct{}
		aprsFiContext           context.Context
		aprsFiCancel            context.CancelFunc
		rfBeaconMu             sync.Mutex
		rfTxMu                 sync.Mutex
		beaconMu               sync.Mutex
		isBeaconMu             sync.Mutex
		beaconWaits            map[string]*beaconWait
		lastRxMu               sync.Mutex
		lastRx                 time.Time
		lastBeaconAttemptMu    sync.Mutex
		lastBeaconAttempt      time.Time
		digiRewriter           *digipeater.Rewriter
		localStationsMu        sync.Mutex
		localStations          map[string]time.Time
		rfMessagesMu           sync.Mutex
		rfMessages             map[string]time.Time
		localRFTxMu            sync.Mutex
		localRFTxPackets       map[string]time.Time
		internetStations       map[string]time.Time
		internetPositions      map[string]*aprs.Packet
		internetPositionTimes  map[string]time.Time
		internetGatedMessages  map[string]time.Time
		lastTrackingPrune      time.Time
		lastInternetMsgPrune   time.Time
		lastMessageRF          time.Time
		messageDedupeMu        sync.Mutex
		messageDedupe          map[string]time.Time
		pendingGatedMessages   map[string]pendingGatedMessage
	}
)

type beaconWait struct {
	frame   string
	payload string
	started time.Time
	result  chan bool
}

type pendingGatedMessage struct {
	packet      *aprs.Packet
	destination string
	due         time.Time
}

type beaconOutcome int

const (
	beaconOutcomeSuccess beaconOutcome = iota
	beaconOutcomeCollision
	beaconOutcomeTimeout
	beaconOutcomeStopped
)

const forwarderQueueSize = 32
const defaultAprsFiBaseURL = "https://api.aprs.fi"
const aprsFiGracePeriod = 10 * time.Second
const localRFEchoWindow = 10 * time.Second

var rfPathComponentPattern = regexp.MustCompile(`^[A-Z0-9]{1,6}(?:-[A-Z0-9]{1,2})?$`)

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
	if cfg.MessageGating {
		if err := validateMessageRFPath(cfg.MessageRFPath); err != nil {
			return nil, err
		}
	}
	aprsis, err := aprs.New(cfg.Aprsis, callSign, cfg.Beacon.Comment, logger)
	if err != nil {
		return nil, fmt.Errorf("Error creating APRS client: %v", err)
	}

	aprsFiContext, aprsFiCancel := context.WithCancel(context.Background())
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
	rangeFilter, err := parseRangeFilter(cfg.Aprsis.Filter)
	if err != nil {
		logger.Warn("Failed to parse APRS-IS range filter: ", err)
	}

	ig := &IGate{
		cfg:                    cfg,
		callSign:               callSign,
		inputChan:              inputChan,
		enableTx:               enableTx,
		tx:                     tx,
		logger:                 logger,
		Aprsis:                 aprsis,
		aprsisUpload:           aprsis.Upload,
		httpClient:             httpClient,
		verifyAprsFi:           verifyAprsFi,
		aprsFiKey:              apiKey,
		aprsFiDelay:            cfg.Beacon.AprsFi.Delay,
		aprsFiTries:            cfg.Beacon.AprsFi.MaxAttempts,
		aprsFiBaseURL:          defaultAprsFiBaseURL,
		aprsFiWatchdogInterval: cfg.Beacon.AprsFi.WatchdogInterval,
		aprsFiWatchdogMaxAge:   cfg.Beacon.AprsFi.WatchdogMaxAge,
		disableISBeacon:        disableISBeacon,
		maxRfAttempts:          cfg.Beacon.MaxRFAttempts,
		geoFilter:              rangeFilter,
		forwardChan:            make(chan *aprs.Packet, forwarderQueueSize),
		stop:                   make(chan struct{}),
		aprsFiContext:          aprsFiContext,
		aprsFiCancel:           aprsFiCancel,
		lastRx:                 time.Now(),
		beaconWaits:            make(map[string]*beaconWait),
		localStations:          make(map[string]time.Time),
		rfMessages:             make(map[string]time.Time),
		localRFTxPackets:       make(map[string]time.Time),
		internetStations:       make(map[string]time.Time),
		internetPositions:      make(map[string]*aprs.Packet),
		internetPositionTimes:  make(map[string]time.Time),
		internetGatedMessages:  make(map[string]time.Time),
		messageDedupe:          make(map[string]time.Time),
		pendingGatedMessages:   make(map[string]pendingGatedMessage),
	}

	return ig, nil
}

func validateMessageRFPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	components := strings.Split(path, ",")
	if len(components) > 8 {
		return fmt.Errorf("message-rf-path contains too many path components")
	}
	for _, raw := range components {
		component := strings.ToUpper(strings.TrimSpace(raw))
		if component == "" {
			return fmt.Errorf("message-rf-path contains an empty component")
		}
		if component == "I" || strings.HasPrefix(component, "Q") || strings.HasPrefix(component, "TCPIP") || strings.HasPrefix(component, "TCPXX") {
			return fmt.Errorf("message-rf-path contains APRS-IS-only component %q", raw)
		}
		if strings.Contains(component, "*") || !rfPathComponentPattern.MatchString(component) {
			return fmt.Errorf("message-rf-path contains invalid AX.25 path component %q", raw)
		}
	}
	return nil
}

func (i *IGate) Run() error {
	messageGatingActive := i.cfg.MessageGating && i.enableTx && i.tx != nil && i.Aprsis != nil
	if i.Aprsis != nil {
		i.Aprsis.SetInboundEnabled(messageGatingActive)
		defer i.Aprsis.SetInboundEnabled(false)
	}

	if i.cfg.Beacon.Enabled {
		err := i.startBeacon()
		if err != nil {
			return fmt.Errorf("Error starting beacon: %v", err)
		}
	}

	var g errgroup.Group

	if i.aprsFiWatchdogEnabled() {
		g.Go(func() error {
			i.runAprsFiWatchdog()
			return nil
		})
	}

	g.Go(func() error {
		return i.forwardPackets()
	})

	g.Go(func() error {
		return i.listenForMessages()
	})

	if messageGatingActive {
		g.Go(func() error {
			return i.gateMessagesToRF()
		})
	}

	return g.Wait()
}

// launchAsync registers independent iGate work before creating a goroutine.
// The shared lifecycle lock prevents Add from racing with shutdown Wait.
func (i *IGate) launchAsync(work func()) bool {
	i.asyncMu.Lock()
	defer i.asyncMu.Unlock()
	select {
	case <-i.stop:
		return false
	default:
	}
	i.asyncWorkers.Add(1)
	go func() {
		defer i.asyncWorkers.Done()
		work()
	}()
	return true
}

func (i *IGate) Stop() {
	i.stopOnce.Do(func() {
		i.asyncMu.Lock()
		close(i.stop)
		i.asyncMu.Unlock()
		if i.aprsFiCancel != nil {
			i.aprsFiCancel()
		}
		if i.Aprsis != nil {
			i.Aprsis.Stop()
		}
		i.cancelPendingBeacon()
	})
	// Join verification and beacon work after cancellation, without holding
	// the admission lock. Repeated Stop calls are safe.
	i.asyncWorkers.Wait()
}

func (i *IGate) SetDigiRewriter(rewriter *digipeater.Rewriter) {
	i.digiRewriter = rewriter
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

			packet, err := aprs.ParsePacket(msg)
			if err != nil {
				i.logger.Error(err, "Could not parse APRS packet: ", msg)
				continue
			}
			if i.isRecentLocalRFEcho(packet) {
				i.logger.Debug("Suppressing APRS-IS upload of locally digipeated RF echo: ", msg)
				continue
			}
			if packet.HasForbiddenRFPath() {
				i.logger.Debug("Skipping RF packet with forbidden path marker: ", msg)
				continue
			}
			i.rememberRFMessageFromRFPacket(packet)

			wasThirdParty := packet.Type() == aprs.ThirdPartyTraffic
			packet, ok = packet.UnwrapThirdPartyForAprsIs()
			if !ok {
				i.logger.Debug("Skipping APRS-IS-originated or malformed third-party packet: ", msg)
				continue
			}
			if !wasThirdParty && isDirectRFPacket(packet) {
				i.rememberLocalStation(packet.Src)
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

			shouldForward := packet.Type().ForwardToAprsIs()
			if shouldForward {
				if !selfPacket && !i.allowForward(packet) {
					i.logger.Debug("Dropping APRS-IS forwarding outside geo filter: ", msg)
					continue
				}
				if selfPacket {
					i.logger.Debug("Forwarding self-originated RF packet to APRS-IS: ", msg)
				}
				forwardPacket := packet
				if i.cfg.GateDigipeatedPath && i.digiRewriter != nil && i.hasSelfHop(packet) {
					packetCopy := *packet
					packetCopy.Path = append([]string{}, packet.Path...)
					if i.digiRewriter.Rewrite(&packetCopy) {
						forwardPacket = &packetCopy
					}
				}
				select {
				case <-i.stop:
					return nil
				case i.forwardChan <- forwardPacket:
				}
			}

			i.observeBeacon(packet)
		}
	}
}

func isDirectRFPacket(packet *aprs.Packet) bool {
	if packet == nil {
		return false
	}

	for _, component := range packet.Path {
		if strings.Contains(component, "*") {
			return false
		}
	}

	return true
}

func (i *IGate) rememberLocalStation(callsign string) {
	callsign = strings.ToUpper(strings.TrimSpace(callsign))
	if callsign == "" {
		return
	}

	i.localStationsMu.Lock()
	i.pruneTrackingLocked(time.Now())
	if i.localStations == nil {
		i.localStations = make(map[string]time.Time)
	}
	i.localStations[callsign] = time.Now()
	i.localStationsMu.Unlock()
}

func (i *IGate) localStationRecentlyHeard(callsign string) bool {
	callsign = strings.ToUpper(strings.TrimSpace(callsign))
	if callsign == "" {
		return false
	}

	i.localStationsMu.Lock()
	defer i.localStationsMu.Unlock()
	i.pruneTrackingLocked(time.Now())

	heard, ok := i.localStations[callsign]
	if !ok || time.Since(heard) > i.cfg.LocalStationTimeout {
		if ok {
			delete(i.localStations, callsign)
		}
		return false
	}

	return true
}

func (i *IGate) rememberRFMessage(packet *aprs.Packet) {
	if packet == nil {
		return
	}
	destination, ok := packet.MessageDestination()
	if !ok {
		return
	}
	key := messageKey(packet, destination)
	now := time.Now()
	window := i.cfg.MessageDedupWindow
	if window <= 0 {
		window = time.Minute
	}

	i.rfMessagesMu.Lock()
	defer i.rfMessagesMu.Unlock()
	if i.rfMessages == nil {
		i.rfMessages = make(map[string]time.Time)
	}
	for cachedKey, seen := range i.rfMessages {
		if now.Sub(seen) >= window {
			delete(i.rfMessages, cachedKey)
		}
	}
	i.rfMessages[key] = now
}

// rememberRFMessageFromRFPacket also tracks Internet-originated messages that
// another i-gate has already wrapped and transmitted over RF. The wrapper's
// inner TCPIP path identifies that format; other third-party packets are not
// treated as evidence that the original message was heard locally.
func (i *IGate) rememberRFMessageFromRFPacket(packet *aprs.Packet) {
	if packet == nil {
		return
	}
	if packet.Type() != aprs.ThirdPartyTraffic {
		i.rememberRFMessage(packet)
		return
	}

	inner, err := aprs.ParsePacket(strings.TrimPrefix(packet.Payload, "}"))
	if err != nil || inner.Type() != aprs.Message || !hasInternetOrigin(inner.Path) {
		return
	}
	i.rememberRFMessage(inner)
}

func (i *IGate) rfMessageRecentlyHeard(key string) bool {
	window := i.cfg.MessageDedupWindow
	if window <= 0 {
		window = time.Minute
	}
	i.rfMessagesMu.Lock()
	defer i.rfMessagesMu.Unlock()
	return i.rfMessageRecentlyHeardLocked(key, time.Now(), window)
}

func (i *IGate) rfMessageRecentlyHeardLocked(key string, now time.Time, window time.Duration) bool {
	for cachedKey, seen := range i.rfMessages {
		if now.Sub(seen) >= window {
			delete(i.rfMessages, cachedKey)
		}
	}
	seen, ok := i.rfMessages[key]
	return ok && now.Sub(seen) < window
}

func (i *IGate) rememberInternetStation(packet *aprs.Packet) {
	if packet == nil {
		return
	}
	callsign := strings.ToUpper(strings.TrimSpace(packet.Src))
	if callsign == "" || strings.EqualFold(callsign, i.callSign) {
		return
	}
	i.localStationsMu.Lock()
	defer i.localStationsMu.Unlock()
	i.pruneTrackingLocked(time.Now())
	if i.internetStations == nil {
		i.internetStations = make(map[string]time.Time)
	}
	i.internetStations[callsign] = time.Now()
	if packet.Type() == aprs.PositionReport {
		if i.internetPositions == nil {
			i.internetPositions = make(map[string]*aprs.Packet)
		}
		copy := *packet
		copy.Path = append([]string(nil), packet.Path...)
		i.internetPositions[callsign] = &copy
		if i.internetPositionTimes == nil {
			i.internetPositionTimes = make(map[string]time.Time)
		}
		i.internetPositionTimes[callsign] = time.Now()
	}
}

func (i *IGate) pruneTrackingLocked(now time.Time) {
	if !i.lastTrackingPrune.IsZero() && now.Sub(i.lastTrackingPrune) < time.Minute {
		return
	}
	i.lastTrackingPrune = now
	timeout := i.cfg.LocalStationTimeout
	if timeout <= 0 {
		timeout = time.Hour
	}
	cutoff := now.Add(-timeout)
	for callsign, heard := range i.localStations {
		if heard.Before(cutoff) {
			delete(i.localStations, callsign)
		}
	}
	for callsign, heard := range i.internetStations {
		if heard.Before(cutoff) {
			delete(i.internetStations, callsign)
			delete(i.internetPositions, callsign)
			delete(i.internetPositionTimes, callsign)
		}
	}
}

func hasInternetOrigin(path []string) bool {
	for _, component := range path {
		component = strings.TrimSuffix(strings.ToUpper(strings.TrimSpace(component)), "*")
		if strings.HasPrefix(component, "TCPIP") || strings.HasPrefix(component, "TCPXX") {
			return true
		}
	}
	return false
}

func messageKey(packet *aprs.Packet, destination string) string {
	return strings.ToUpper(strings.TrimSpace(packet.Src)) + ">" + destination + ":" + packet.Payload
}

func (i *IGate) observeInternetPacket(packet *aprs.Packet) {
	if packet == nil {
		return
	}
	i.rememberInternetStation(packet)
	i.pruneInternetGatedMessages(time.Now())

	if packet.Type() != aprs.ThirdPartyTraffic {
		return
	}
	inner, err := aprs.ParsePacket(strings.TrimPrefix(packet.Payload, "}"))
	if err != nil || !hasInternetOrigin(inner.Path) {
		return
	}

	// A third-party packet with TCPIP/TCPXX in its inner path is evidence that
	// the inner station has already been gated by another Internet gateway.
	i.rememberInternetStation(inner)
	if destination, ok := inner.MessageDestination(); ok {
		now := time.Now()
		if i.internetGatedMessages == nil {
			i.internetGatedMessages = make(map[string]time.Time)
		}
		if i.pendingGatedMessages == nil {
			i.pendingGatedMessages = make(map[string]pendingGatedMessage)
		}
		i.internetGatedMessages[messageKey(inner, destination)] = now
		delete(i.pendingGatedMessages, messageKey(inner, destination))
	}
}

func (i *IGate) pruneInternetGatedMessages(now time.Time) {
	if !i.lastInternetMsgPrune.IsZero() && now.Sub(i.lastInternetMsgPrune) < time.Minute {
		return
	}
	i.lastInternetMsgPrune = now
	window := i.cfg.MessageDedupWindow
	if window <= 0 {
		window = time.Minute
	}
	for key, seen := range i.internetGatedMessages {
		if now.Sub(seen) >= window {
			delete(i.internetGatedMessages, key)
		}
	}
}

func (i *IGate) internetStationRecentlyHeard(callsign string) bool {
	callsign = strings.ToUpper(strings.TrimSpace(callsign))
	i.localStationsMu.Lock()
	defer i.localStationsMu.Unlock()
	i.pruneTrackingLocked(time.Now())
	heard, ok := i.internetStations[callsign]
	if !ok || time.Since(heard) > i.cfg.LocalStationTimeout {
		if ok {
			delete(i.internetStations, callsign)
			delete(i.internetPositions, callsign)
			delete(i.internetPositionTimes, callsign)
		}
		return false
	}
	return true
}

func (i *IGate) recentInternetPosition(callsign string) *aprs.Packet {
	i.localStationsMu.Lock()
	defer i.localStationsMu.Unlock()
	i.pruneTrackingLocked(time.Now())
	callsign = strings.ToUpper(strings.TrimSpace(callsign))
	if time.Since(i.internetPositionTimes[callsign]) > i.cfg.LocalStationTimeout {
		delete(i.internetPositions, callsign)
		delete(i.internetPositionTimes, callsign)
		return nil
	}
	return i.internetPositions[callsign]
}

func (i *IGate) alreadyGatedMessage(packet *aprs.Packet, destination string) bool {
	if packet == nil {
		return true
	}

	window := i.cfg.MessageDedupWindow
	if window <= 0 {
		window = time.Minute
	}
	key := messageKey(packet, destination)
	now := time.Now()

	i.messageDedupeMu.Lock()
	defer i.messageDedupeMu.Unlock()
	if i.messageDedupe == nil {
		i.messageDedupe = make(map[string]time.Time)
	}
	for cachedKey, seen := range i.messageDedupe {
		if now.Sub(seen) >= window {
			delete(i.messageDedupe, cachedKey)
		}
	}
	if seen, ok := i.messageDedupe[key]; ok && now.Sub(seen) < window {
		return true
	}
	i.messageDedupe[key] = now
	return false
}

func (i *IGate) internetGatedMessageRecentlySeen(key string) bool {
	seen, ok := i.internetGatedMessages[key]
	if !ok {
		return false
	}
	window := i.cfg.MessageDedupWindow
	if window <= 0 {
		window = time.Minute
	}
	if time.Since(seen) >= window {
		delete(i.internetGatedMessages, key)
		return false
	}
	return true
}

func (i *IGate) gateMessagesToRF() error {
	for {
		var timer *time.Timer
		var timerC <-chan time.Time
		var dueKey string
		var due time.Time
		for key, pending := range i.pendingGatedMessages {
			if dueKey == "" || pending.due.Before(due) {
				dueKey = key
				due = pending.due
			}
		}
		if dueKey != "" {
			wait := time.Until(due)
			if wait < 0 {
				wait = 0
			}
			timer = time.NewTimer(wait)
			timerC = timer.C
		}

		select {
		case <-i.stop:
			if timer != nil {
				timer.Stop()
			}
			return nil
		case <-timerC:
			pending, ok := i.pendingGatedMessages[dueKey]
			if !ok {
				continue
			}
			if i.internetGatedMessageRecentlySeen(dueKey) {
				delete(i.pendingGatedMessages, dueKey)
				continue
			}
			if i.internetStationRecentlyHeard(pending.destination) {
				delete(i.pendingGatedMessages, dueKey)
				continue
			}
			if i.cfg.MessageRFInterval > 0 && !i.lastMessageRF.IsZero() {
				wait := i.cfg.MessageRFInterval - time.Since(i.lastMessageRF)
				if wait > 0 {
					pending.due = time.Now().Add(wait)
					i.pendingGatedMessages[dueKey] = pending
					continue
				}
			}
			frame := formatThirdPartyForRF(pending.packet, i.callSign, i.cfg.MessageRFPath)
			i.logger.Info("Gating APRS-IS message to RF: ", frame)
			frames := []string{frame}
			if position := i.recentInternetPosition(pending.packet.Src); position != nil {
				frames = append([]string{formatThirdPartyForRF(position, i.callSign, i.cfg.MessageRFPath)}, frames...)
			}
			sent, duplicate := i.sendRFFramesUnlessRFMessage(frames, dueKey)
			if sent {
				i.lastMessageRF = time.Now()
			} else if duplicate {
				i.logger.Debug("Skipping APRS-IS message already heard on RF: ", dueKey)
			}
			delete(i.pendingGatedMessages, dueKey)
		case msg, ok := <-i.Aprsis.Inbound:
			if !ok {
				return nil
			}

			packet, err := aprs.ParsePacket(msg)
			if err != nil {
				continue
			}
			i.observeInternetPacket(packet)
			if packet.Type() != aprs.Message {
				continue
			}

			if packet.Payload[0] == '}' || strings.EqualFold(packet.Src, i.callSign) {
				continue
			}

			destination, ok := packet.MessageDestination()
			if !ok || !i.localStationRecentlyHeard(destination) || i.localStationRecentlyHeard(packet.Src) || i.internetStationRecentlyHeard(destination) {
				continue
			}

			if strings.EqualFold(destination, i.callSign) || hasNoGateMarker(packet) {
				continue
			}
			if i.alreadyGatedMessage(packet, destination) {
				i.logger.Debug("Skipping duplicate APRS-IS message: ", msg)
				continue
			}
			key := messageKey(packet, destination)
			if i.rfMessageRecentlyHeard(key) {
				i.logger.Debug("Skipping APRS-IS message already heard on RF: ", key)
				continue
			}
			if i.internetGatedMessageRecentlySeen(key) {
				continue
			}
			if i.pendingGatedMessages == nil {
				i.pendingGatedMessages = make(map[string]pendingGatedMessage)
			}
			i.pendingGatedMessages[key] = pendingGatedMessage{
				packet:      packet,
				destination: destination,
				due:         time.Now().Add(i.cfg.MessageGateDelay),
			}
		}
	}
}

func hasNoGateMarker(packet *aprs.Packet) bool {
	if packet == nil {
		return true
	}

	for _, component := range packet.Path {
		component = strings.TrimSuffix(strings.ToUpper(strings.TrimSpace(component)), "*")
		if component == "NOGATE" || component == "RFONLY" || component == "TCPXX" {
			return true
		}
	}

	return false
}

// SendRF queues an RF frame immediately. RF digipeats have already been
// received locally and should not wait for the beacon channel-quiet policy.
func (i *IGate) SendRF(frame string) bool {
	if !i.sendRFFrames([]string{frame}) {
		return false
	}
	if packet, err := aprs.ParsePacket(frame); err == nil {
		i.rememberLocalRFTx(packet)
	}
	return true
}

func localRFPacketKey(packet *aprs.Packet) string {
	if packet == nil {
		return ""
	}
	path := make([]string, len(packet.Path))
	for idx, hop := range packet.Path {
		path[idx] = strings.ToUpper(strings.TrimSpace(hop))
	}
	return strings.ToUpper(strings.TrimSpace(packet.Src)) + ">" +
		strings.ToUpper(strings.TrimSpace(packet.Dst)) + "," + strings.Join(path, ",") + ":" + packet.Payload
}

func (i *IGate) rememberLocalRFTx(packet *aprs.Packet) {
	key := localRFPacketKey(packet)
	if key == "" {
		return
	}
	now := time.Now()
	i.localRFTxMu.Lock()
	defer i.localRFTxMu.Unlock()
	if i.localRFTxPackets == nil {
		i.localRFTxPackets = make(map[string]time.Time)
	}
	for cachedKey, sent := range i.localRFTxPackets {
		if now.Sub(sent) >= localRFEchoWindow {
			delete(i.localRFTxPackets, cachedKey)
		}
	}
	i.localRFTxPackets[key] = now
}

func (i *IGate) isRecentLocalRFEcho(packet *aprs.Packet) bool {
	key := localRFPacketKey(packet)
	if key == "" {
		return false
	}
	now := time.Now()
	i.localRFTxMu.Lock()
	defer i.localRFTxMu.Unlock()
	for cachedKey, sent := range i.localRFTxPackets {
		if now.Sub(sent) >= localRFEchoWindow {
			delete(i.localRFTxPackets, cachedKey)
		}
	}
	sent, ok := i.localRFTxPackets[key]
	return ok && now.Sub(sent) < localRFEchoWindow
}

func (i *IGate) sendRFFrames(frames []string) bool {
	if i.tx == nil || len(frames) == 0 {
		return false
	}
	i.rfTxMu.Lock()
	defer i.rfTxMu.Unlock()
	return i.queueRFFrames(frames)
}

// sendRFFramesUnlessRFMessage makes the duplicate check at RF queue admission,
// under the same lock that serializes frame admission with other RF sends.
func (i *IGate) sendRFFramesUnlessRFMessage(frames []string, key string) (sent, duplicate bool) {
	if i.tx == nil || len(frames) == 0 {
		return false, false
	}
	window := i.cfg.MessageDedupWindow
	if window <= 0 {
		window = time.Minute
	}
	for _, frame := range frames {
		for {
			i.rfTxMu.Lock()
			// The queue attempt is nonblocking, so checking and queueing while
			// holding the RF-message lock leaves reception free whenever space is
			// unavailable.
			i.rfMessagesMu.Lock()
			if i.rfMessageRecentlyHeardLocked(key, time.Now(), window) {
				i.rfMessagesMu.Unlock()
				i.rfTxMu.Unlock()
				return false, true
			}
			queued := i.tx.TrySend(frame)
			i.rfMessagesMu.Unlock()
			i.rfTxMu.Unlock()
			if queued {
				break
			}
			if !i.waitForTransmitQueueSlot() {
				return false, false
			}
		}
	}
	return true, false
}

func (i *IGate) sendRFFramesWhenQuiet(frames []string, onAdmission ...func()) bool {
	if i.tx == nil || len(frames) == 0 {
		return false
	}
	waitStarted := time.Now()
	for _, frame := range frames {
		for {
			if !i.waitForQuiet(beaconChannelQuiet) {
				return false
			}
			i.rfTxMu.Lock()
			// Hold the receive timestamp only around a nonblocking enqueue. This
			// makes the quiet check current at admission without blocking RX while
			// the transmitter queue is full.
			i.lastRxMu.Lock()
			quiet := i.lastRx.IsZero() || time.Since(i.lastRx) >= beaconChannelQuiet
			queued := quiet && i.tx.TrySend(frame)
			if queued && len(onAdmission) > 0 && onAdmission[0] != nil {
				onAdmission[0]()
			}
			i.lastRxMu.Unlock()
			i.rfTxMu.Unlock()
			if queued {
				if waited := time.Since(waitStarted); waited >= 250*time.Millisecond {
					i.logger.Debug("Waited ", waited, " for RF channel quiet before queuing ", len(frames), " frame(s)")
				}
				break
			}
			if !quiet {
				continue
			}
			if !i.waitForTransmitQueueSlot() {
				return false
			}
		}
	}
	return true
}

func (i *IGate) waitForTransmitQueueSlot() bool {
	timer := time.NewTimer(10 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-i.stop:
		return false
	case <-timer.C:
		return true
	}
}

func (i *IGate) queueRFFrames(frames []string) bool {
	for _, frame := range frames {
		if !i.tx.SendUntil(frame, i.stop) {
			return false
		}
	}
	return true
}

func formatThirdPartyForRF(packet *aprs.Packet, callSign, rfPath string) string {
	var builder strings.Builder
	builder.WriteString(strings.ToUpper(strings.TrimSpace(callSign)))
	builder.WriteString(">APRS")
	if path := strings.TrimSpace(rfPath); path != "" {
		builder.WriteString(",")
		builder.WriteString(path)
	}
	builder.WriteString(":}")
	builder.WriteString(packet.Src)
	builder.WriteString(">")
	builder.WriteString(packet.Dst)
	builder.WriteString(",TCPIP,")
	builder.WriteString(strings.ToUpper(strings.TrimSpace(callSign)))
	builder.WriteString("*")
	builder.WriteString(":")
	builder.WriteString(packet.Payload)
	return builder.String()
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

			bidirectional := i.cfg.MessageGating && i.enableTx && i.tx != nil &&
				isDirectRFPacket(packet) && i.localStationRecentlyHeard(packet.Src)
			uploadFrame := formatForAprsIs(packet, i.callSign, bidirectional)
			fmt.Printf("uploading APRS-IS packet: %v\n", uploadFrame)
			if i.aprsisUpload != nil {
				if err := i.aprsisUpload(uploadFrame); err != nil {
					i.forwardErrors.Add(1)
					i.logger.Error("Error uploading APRS packet: ", err)
				} else {
					i.forwardWrites.Add(1)
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
		path       string
		interval   time.Duration
		allowRetry bool
	}

	var rfSchedules []rfSchedule

	if !i.cfg.Beacon.DisableRF {
		if rfInterval > 0 {
			rfSchedules = append(rfSchedules, rfSchedule{
				path:       strings.TrimSpace(i.cfg.Beacon.RFPath),
				interval:   rfInterval,
				allowRetry: true,
			})
		}

		if len(extraRF) > 0 {
			i.logger.Warn("Ignoring additional-rf-beacons entries; only primary RF schedule is supported")
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

	sendRF := func(path string, allowRetry bool, reason string) {
		if i.cfg.Beacon.DisableRF || !i.enableTx || i.tx == nil {
			return
		}

		if chainIsToRf {
			sendAprsIs(reason)
		}

		rfFrame := buildBeaconFrame(i.callSign, path, i.cfg.Beacon.Comment)
		i.logger.Info("Beacon -> RF (", reason, "): ", rfFrame)
		i.launchAsync(func() { i.sendBeaconRf(rfFrame, i.cfg.Beacon.Comment, allowRetry) })
	}

	// Send initial beacon to each configured destination
	if isInterval > 0 && !chainIsToRf {
		sendAprsIs("initial")
	}

	for _, schedule := range rfSchedules {
		sendRF(schedule.path, schedule.allowRetry, "initial")
	}

	if isTicker != nil {
		if !i.launchAsync(func() {
			defer isTicker.Stop()

			for {
				select {
				case <-i.stop:
					return
				case <-isChan:
					sendAprsIs("scheduled")
				}
			}
		}) {
			isTicker.Stop()
		}
	}

	for _, schedule := range rfSchedules {
		s := schedule
		i.launchAsync(func() {
			ticker := time.NewTicker(s.interval)
			defer ticker.Stop()

			for {
				select {
				case <-i.stop:
					return
				case <-ticker.C:
					sendRF(s.path, s.allowRetry, "scheduled")
				}
			}
		})
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

func formatForAprsIs(packet *aprs.Packet, callSign string, messageGating bool) string {
	var builder strings.Builder

	callSign = strings.ToUpper(strings.TrimSpace(callSign))

	builder.WriteString(packet.Src)
	builder.WriteString(">")
	builder.WriteString(packet.Dst)

	path := append([]string{}, packet.Path...)
	if !containsAprsIsHop(path) && callSign != "" {
		qConstruct := "qAO"
		if messageGating {
			qConstruct = "qAR"
		}
		path = append(path, qConstruct, callSign)
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

func (i *IGate) channelQuietRemaining(duration time.Duration) time.Duration {
	i.lastRxMu.Lock()
	defer i.lastRxMu.Unlock()

	if i.lastRx.IsZero() {
		return 0
	}
	remaining := duration - time.Since(i.lastRx)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (i *IGate) recordBeaconAttempt(t time.Time) {
	if t.IsZero() {
		t = time.Now()
	}
	i.lastBeaconAttemptMu.Lock()
	i.lastBeaconAttempt = t
	i.lastBeaconAttemptMu.Unlock()
}

func (i *IGate) lastBeaconAttemptTime() (time.Time, bool) {
	i.lastBeaconAttemptMu.Lock()
	defer i.lastBeaconAttemptMu.Unlock()

	if i.lastBeaconAttempt.IsZero() {
		return time.Time{}, false
	}

	return i.lastBeaconAttempt, true
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

		var sendStart time.Time
		var key string
		var outcomeCh <-chan bool
		if !i.sendRFFramesWhenQuiet([]string{frame}, func() {
			// Register before releasing the receive timestamp lock. The receive
			// loop marks activity before observing packets, so an immediate local
			// echo cannot arrive before this expectation exists.
			sendStart = time.Now()
			i.recordBeaconAttempt(sendStart)
			key, outcomeCh = i.expectBeacon(frame, payload, sendStart)
		}) {
			if outcomeCh != nil {
				i.clearBeaconExpectation(key, outcomeCh)
			}
			return
		}

		i.logger.Info("Beacon -> RF: ", frame)

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
			i.logger.Debug("Beacon RF transmission not locally decoded; transmission outcome is unknown")
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
		remaining := i.channelQuietRemaining(duration)
		if remaining == 0 && i.channelQuietFor(duration) {
			return true
		}
		if !i.sleepWithStop(remaining) {
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

func (i *IGate) hasSelfHop(packet *aprs.Packet) bool {
	if packet == nil {
		return false
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

func (i *IGate) ForwardStats() (uint64, uint64) { return i.forwardWrites.Load(), i.forwardErrors.Load() }
