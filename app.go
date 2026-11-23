package main

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/oorrwullie/sparky/internal/cache"
	"github.com/oorrwullie/sparky/internal/capture"
	"github.com/oorrwullie/sparky/internal/config"
	"github.com/oorrwullie/sparky/internal/digipeater"
	"github.com/oorrwullie/sparky/internal/igate"
	"github.com/oorrwullie/sparky/internal/log"
	multimonpackage "github.com/oorrwullie/sparky/internal/multimon"
	openwebrx "github.com/oorrwullie/sparky/internal/openwebrx"
	"github.com/oorrwullie/sparky/internal/pubsub"
	"github.com/oorrwullie/sparky/internal/transmitter"

	"golang.org/x/sync/errgroup"
)

type (
	Sparky struct {
		cfg               config.Config
		captureDevice     capture.Capture
		captureOutputChan chan []byte
		multimon          *multimonpackage.Multimon
		mqttInput         *openwebrx.MQTT
		transmitter       *transmitter.Transmitter
		stop              chan struct{}
		stopOnce          sync.Once
		shutdownComplete  chan struct{}
		igate             *igate.IGate
		digipeater        *digipeater.Digipeater
		logger            *log.Logger
		cache             *cache.Cache
		pubsub            *pubsub.PubSub
	}
)

const minPacketSize = 35

func NewSparky(logger *log.Logger) (*Sparky, error) {
	var (
		tx                *transmitter.Transmitter
		txHandle          *transmitter.Tx
		ig                *igate.IGate
		dp                *digipeater.Digipeater
		captureOutputChan = make(chan []byte, 10)
		multimon          *multimonpackage.Multimon
		mqttInput         *openwebrx.MQTT
		ps                = pubsub.New()
		soundcard         *capture.SoundcardCapture
		deviceType        string
	)

	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	appCache := cache.NewCache(cfg.CacheSize, ".cache.json", 60*time.Second)
	var captureDevice capture.Capture
	if cfg.OpenWebRXMQTT.Enabled {
		mqttInput, err = openwebrx.New(cfg.OpenWebRXMQTT, cfg.MapFlights, ps, appCache, logger)
		if err != nil {
			return nil, fmt.Errorf("Error starting OpenWebRX+ MQTT input: %w", err)
		}
		deviceType = "OpenWebRX+ MQTT"
	} else {
		captureDevice, err = capture.New(cfg, captureOutputChan, logger)
		if err != nil {
			return nil, fmt.Errorf("Error creating capture device: %v", err)
		}
		if captureDevice == nil || isNilCapture(captureDevice) {
			return nil, fmt.Errorf("Error creating capture device: capture backend initialization returned nil; verify the SDR or soundcard settings")
		}
		if err = captureDevice.Start(); err != nil {
			return nil, fmt.Errorf("Error starting SDR: %v", err)
		}
		deviceType = captureDevice.Type()
	}

	if cfg.Transmitter.Enabled {
		if sc, ok := captureDevice.(*capture.SoundcardCapture); ok {
			if sc == nil {
				return nil, fmt.Errorf("capture device reported type %q but returned a nil implementation", deviceType)
			}
			soundcard = sc
		} else if captureDevice != nil {
			if deviceType == "Soundcard" {
				return nil, fmt.Errorf("capture device reported type %q but is %T", deviceType, captureDevice)
			}

			soundcard, err = capture.NewSoundcardCapture(cfg, captureOutputChan, logger)
			if err != nil {
				return nil, fmt.Errorf("Error creating soundcard capture: %v", err)
			}
			go soundcard.Start()
		} else {
			// In MQTT receive mode, create a transmit-only sound-card device.
			soundcard, err = capture.NewSoundcardTransmitter(cfg, captureOutputChan, logger)
			if err != nil {
				return nil, fmt.Errorf("Error creating transmit soundcard: %v", err)
			}
			if err = soundcard.Start(); err != nil {
				return nil, fmt.Errorf("Error starting transmit soundcard: %v", err)
			}
		}

		tx, err = transmitter.New(soundcard, logger)
		if err != nil {
			return nil, fmt.Errorf("Error creating transmitter: %v", err)
		}
		txHandle = tx.Tx
	}

	if captureDevice != nil {
		multimon = multimonpackage.New(cfg.Multimon, captureOutputChan, ps, appCache, txHandle, cfg.StationCallsign, logger)
		if err = multimon.Start(); err != nil {
			return nil, fmt.Errorf("Error starting multimon: %v", err)
		}
	}

	if cfg.IGate.Enabled {
		ig, err = igate.New(cfg.IGate, ps, cfg.Transmitter.Enabled, txHandle, cfg.StationCallsign, logger)
		if err != nil {
			return nil, fmt.Errorf("Error creating IGate client: %v", err)
		}
	}

	var gateRewriter *digipeater.Rewriter
	if cfg.IGate.GateDigipeatedPath {
		if cfg.DigipeaterEnabled {
			gateRewriter, err = digipeater.NewRewriter(cfg.StationCallsign, cfg.Digipeater)
			if err != nil {
				return nil, fmt.Errorf("Error creating digipeater rewriter: %v", err)
			}
		} else {
			logger.Warn("igate.gate-digipeated-path enabled but digipeater is disabled; using original RF path")
		}
	}

	if cfg.DigipeaterEnabled {
		if txHandle == nil {
			logger.Warn("Digipeater enabled but transmitter is disabled; skipping digipeater")
		} else {
			dp, err = digipeater.New(txHandle, ps, cfg.StationCallsign, cfg.Digipeater, logger)
			if err != nil {
				return nil, fmt.Errorf("Error creating digipeater: %v", err)
			}
		}
	}

	if ig != nil && gateRewriter != nil {
		ig.SetDigiRewriter(gateRewriter)
	}
	if dp != nil && deviceType == "Soundcard" {
		dp.SetTransmitDelay(100 * time.Millisecond)
		logger.Info("Using 100ms digipeater delay for sound-card reception")
	}
	if ig != nil && dp != nil {
		dp.SetRFSend(ig.SendRF)
	}

	dg := &Sparky{
		cfg:               cfg,
		captureOutputChan: captureOutputChan,
		transmitter:       tx,
		stop:              make(chan struct{}),
		shutdownComplete:  make(chan struct{}),
		igate:             ig,
		logger:            logger,
		cache:             appCache,
		multimon:          multimon,
		mqttInput:         mqttInput,
		captureDevice:     captureDevice,
		digipeater:        dp,
	}

	return dg, nil
}

func (d *Sparky) Run() error {
	// Arm cleanup before starting workers so startup failures also stop resources.
	go func() {
		<-d.stop
		var workers sync.WaitGroup
		stop := func(name string, fn func()) {
			workers.Add(1)
			go func() {
				defer workers.Done()
				d.logger.Info("Stopping ", name)
				fn()
			}()
		}

		// Independent services can stop concurrently; no slow network shutdown
		// should delay stopping the receiver or transmitter.
		if d.igate != nil {
			stop("iGate", d.igate.Stop)
		}
		if d.digipeater != nil {
			stop("digipeater", d.digipeater.Stop)
		}
		if d.captureDevice != nil {
			stop("capture device", d.captureDevice.Stop)
		}
		if d.multimon != nil {
			stop("multimon-ng", d.multimon.Stop)
		}
		if d.mqttInput != nil {
			stop("OpenWebRX+ MQTT input", d.mqttInput.Stop)
		}
		if d.transmitter != nil {
			stop("transmitter", d.transmitter.Stop)
		}
		workers.Wait()
		close(d.shutdownComplete)
	}()
	defer func() {
		d.Stop()
		<-d.shutdownComplete
	}()

	if d.transmitter != nil {
		if err := d.transmitter.Start(); err != nil {
			return fmt.Errorf("starting transmitter: %w", err)
		}
	}

	var g errgroup.Group
	// An unexpected return from either service, including an error, must
	// request cancellation so the other long-running service can exit.
	if d.igate != nil {
		g.Go(func() error {
			defer d.Stop()
			return d.igate.Run()
		})
	}
	if d.digipeater != nil {
		g.Go(func() error {
			defer d.Stop()
			return d.digipeater.Run()
		})
	} else if d.cfg.DigipeaterEnabled {
		d.logger.Warn("Digipeater enabled but transmitter is disabled; digipeater will not run")
	}

	if d.igate == nil && d.digipeater == nil {
		<-d.stop // MQTT-only and disabled-worker configurations stay alive.
		return nil
	}
	return g.Wait()
}

// Stop signals cancellation immediately and is safe to call repeatedly.
func (d *Sparky) Stop() {
	d.stopOnce.Do(func() { close(d.stop) })
}

func isNilCapture(c capture.Capture) bool {
	val := reflect.ValueOf(c)
	if !val.IsValid() {
		return true
	}

	switch val.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Interface, reflect.Slice, reflect.Func, reflect.Chan:
		return val.IsNil()
	default:
		return false
	}
}
