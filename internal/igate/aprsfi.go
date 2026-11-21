package igate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// APRS.fi monitoring and verification remain asynchronous to RF processing.
// At most two verification polls are pending concurrently; others are skipped
// rather than accumulating unbounded timers and HTTP requests.
const maxConcurrentAprsFiVerifications = 2
func (i *IGate) aprsFiWatchdogEnabled() bool {
	if !i.verifyAprsFi {
		return false
	}
	if i.aprsFiWatchdogInterval <= 0 || i.aprsFiWatchdogMaxAge <= 0 {
		return false
	}
	if i.httpClient == nil || strings.TrimSpace(i.aprsFiKey) == "" {
		return false
	}
	if i.cfg.Beacon.DisableRF || !i.enableTx || i.tx == nil {
		return false
	}
	if strings.TrimSpace(i.callSign) == "" {
		return false
	}

	return true
}

func (i *IGate) runAprsFiWatchdog() {
	interval := i.aprsFiWatchdogInterval
	maxAge := i.aprsFiWatchdogMaxAge

	if interval <= 0 || maxAge <= 0 {
		return
	}

	i.logger.Info("Starting APRS.fi watchdog every ", interval, " (max-age ", maxAge, ")")
	i.aprsFiWatchdogTick(maxAge)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-i.stop:
			return
		case <-ticker.C:
			i.aprsFiWatchdogTick(maxAge)
		}
	}
}

func (i *IGate) watchdogLocalGap() time.Duration {
	gap := i.aprsFiWatchdogInterval / 2
	if gap <= 0 {
		gap = 5 * time.Minute
	}
	return gap
}

func (i *IGate) aprsFiWatchdogTick(maxAge time.Duration) {
	if maxAge <= 0 {
		return
	}
	select {
	case <-i.stop:
		return
	default:
	}

	lastAttempt, hasAttempt := i.lastBeaconAttemptTime()
	attemptAge := time.Duration(0)
	if hasAttempt {
		attemptAge = time.Since(lastAttempt)
	}
	localGap := i.watchdogLocalGap()
	allowLocalRetry := !hasAttempt || attemptAge >= localGap

	latest, err := i.latestAprsFiTimestamp()
	if err != nil {
		i.logger.Warn("APRS.fi watchdog query failed: ", err)
		if allowLocalRetry && (!hasAttempt || attemptAge >= maxAge) {
			i.logger.Warn("APRS.fi watchdog falling back to local timer; last beacon attempt ", attemptAge, " ago")
			i.triggerWatchdogBeacon("aprs.fi query failed")
		}
		return
	}

	if latest.IsZero() {
		if allowLocalRetry {
			i.logger.Warn("APRS.fi watchdog did not find recent packets; last local attempt ", attemptAge, " ago")
			i.triggerWatchdogBeacon("no aprs.fi entries")
		} else {
			i.logger.Info("APRS.fi watchdog found no entries but last beacon attempt ", attemptAge, " ago (< ", localGap, "), skipping")
		}
		return
	}

	age := time.Since(latest)
	if age > maxAge {
		if allowLocalRetry {
			i.logger.Warn("APRS.fi last heard ", age, " ago (max ", maxAge, "); last local attempt ", attemptAge, " ago – triggering RF beacon")
			i.triggerWatchdogBeacon("aprs.fi stale entry")
		} else {
			i.logger.Warn("APRS.fi stale (", age, ") but last local attempt ", attemptAge, " ago (< ", localGap, "); waiting before retry")
		}
		return
	}

	if hasAttempt && lastAttempt.After(latest.Add(aprsFiGracePeriod)) && allowLocalRetry && attemptAge > maxAge/2 {
		i.logger.Warn("APRS.fi has not seen the most recent beacon (local age ", attemptAge, ", aprs.fi age ", age, "); triggering RF beacon")
		i.triggerWatchdogBeacon("aprs.fi missing recent beacon")
		return
	}

	i.logger.Debug("APRS.fi watchdog healthy; aprs.fi age ", age, ", local beacon age ", attemptAge)
}

func (i *IGate) triggerWatchdogBeacon(reason string) {
	if i.cfg.Beacon.DisableRF || !i.enableTx || i.tx == nil {
		return
	}

	frame := buildBeaconFrame(i.callSign, i.cfg.Beacon.RFPath, i.cfg.Beacon.Comment)
	if reason != "" {
		i.logger.Warn("APRS.fi watchdog -> RF (", reason, "): ", frame)
	} else {
		i.logger.Warn("APRS.fi watchdog -> RF: ", frame)
	}
	i.launchAsync(func() { i.sendBeaconRf(frame, i.cfg.Beacon.Comment, true) })
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

	// Register a verification before launching it; shutdown and admission
	// share the lock so Stop can safely wait for all registered work.
	i.asyncMu.Lock()
	select {
	case <-i.stop:
		i.asyncMu.Unlock()
		return
	default:
	}
	if i.verificationSlots == nil {
		i.verificationSlots = make(chan struct{}, maxConcurrentAprsFiVerifications)
	}
	select {
	case i.verificationSlots <- struct{}{}:
	default:
		i.asyncMu.Unlock()
		i.logger.Warn("Skipping APRS.fi verification: all verification slots busy")
		return
	}
	slots := i.verificationSlots
	i.asyncWorkers.Add(1)
	i.asyncMu.Unlock()
	go func() {
		defer i.asyncWorkers.Done()
		defer func() { <-slots }()
		i.runAprsFiVerification(frame, payload, sent, allowRetry)
	}()
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
		select {
		case <-i.stop:
			return
		default:
		}
		i.logger.Warn("APRS.fi did not confirm beacon; retrying RF beacon")
		// This retry is joined with the verification worker, not detached.
		i.sendBeaconRf(frame, payload, false)
		return
	}

	//i.logger.Warn("APRS.fi did not confirm beacon after RF retry; forcing APRS-IS upload")
	//i.forceAprsIsBeacon()
}

func (i *IGate) latestAprsFiTimestamp() (time.Time, error) {
	if i.httpClient == nil {
		return time.Time{}, fmt.Errorf("aprs.fi HTTP client not configured")
	}
	if strings.TrimSpace(i.aprsFiKey) == "" {
		return time.Time{}, fmt.Errorf("aprs.fi api-key missing")
	}
	if strings.TrimSpace(i.callSign) == "" {
		return time.Time{}, fmt.Errorf("aprs.fi call-sign missing")
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

	ctx := i.aprsFiContext
	if ctx == nil {
		ctx = context.Background() // For manually constructed test instances.
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return time.Time{}, err
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return time.Time{}, fmt.Errorf("aprs.fi status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return time.Time{}, err
	}

	var result aprsFiResponse
	if err := json.Unmarshal(payload, &result); err != nil {
		return time.Time{}, err
	}
	if !strings.EqualFold(result.Result, "ok") {
		if result.Description != "" {
			return time.Time{}, fmt.Errorf("aprs.fi error: %s", result.Description)
		}
		return time.Time{}, fmt.Errorf("aprs.fi returned result %q", result.Result)
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

	return latest, nil
}

func (i *IGate) queryAprsFi(sent time.Time) (bool, error) {
	if sent.IsZero() {
		return false, fmt.Errorf("sent time not provided")
	}

	latest, err := i.latestAprsFiTimestamp()
	if err != nil {
		return false, err
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
