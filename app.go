package main

import (
	"fmt"
	"reflect"

	"github.com/oorrwullie/go-igate/internal/cache"
	"github.com/oorrwullie/go-igate/internal/capture"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/digipeater"
	"github.com/oorrwullie/go-igate/internal/igate"
	"github.com/oorrwullie/go-igate/internal/log"
	multimonpackage "github.com/oorrwullie/go-igate/internal/multimon"
	"github.com/oorrwullie/go-igate/internal/pubsub"
	"github.com/oorrwullie/go-igate/internal/transmitter"

	"golang.org/x/sync/errgroup"
)

type (
	DigiGate struct {
		cfg               config.Config
		captureDevice     capture.Capture
		captureOutputChan chan []byte
		multimon          *multimonpackage.Multimon
		transmitter       *transmitter.Transmitter
		stop              chan bool
		igate             *igate.IGate
		digipeater        *digipeater.Digipeater
		logger            *log.Logger
		cache             *cache.Cache
		pubsub            *pubsub.PubSub
	}
)

const minPacketSize = 35

func NewDigiGate(logger *log.Logger) (*DigiGate, error) {
	var (
		tx                *transmitter.Transmitter
		txHandle          *transmitter.Tx
		ig                *igate.IGate
		dp                *digipeater.Digipeater
		captureOutputChan = make(chan []byte, 10)
		multimon          *multimonpackage.Multimon
		ps                = pubsub.New()
		soundcard         *capture.SoundcardCapture
	)

	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	captureDevice, err := capture.New(cfg, captureOutputChan, logger)
	if err != nil {
		return nil, fmt.Errorf("Error creating capture device: %v", err)
	}
	if captureDevice == nil {
		return nil, fmt.Errorf("Error creating capture device: capture backend initialization returned nil; verify the SDR or soundcard settings")
	}

	if isNilCapture(captureDevice) {
		return nil, fmt.Errorf("Error creating capture device: %T returned a nil pointer; verify the SDR or soundcard settings", captureDevice)
	}

	err = captureDevice.Start()
	if err != nil {
		return nil, fmt.Errorf("Error starting SDR: %v", err)
	}

	deviceType := captureDevice.Type()

	if cfg.Transmitter.Enabled {
		if sc, ok := captureDevice.(*capture.SoundcardCapture); ok {
			if sc == nil {
				return nil, fmt.Errorf("capture device reported type %q but returned a nil implementation", deviceType)
			}
			soundcard = sc
		} else {
			if deviceType == "Soundcard" {
				return nil, fmt.Errorf("capture device reported type %q but is %T", deviceType, captureDevice)
			}

			soundcard, err = capture.NewSoundcardCapture(cfg, captureOutputChan, logger)
			if err != nil {
				return nil, fmt.Errorf("Error creating soundcard capture: %v", err)
			}
			go soundcard.Start()
		}

		tx, err = transmitter.New(soundcard, logger)
		if err != nil {
			return nil, fmt.Errorf("Error creating transmitter: %v", err)
		}
		txHandle = tx.Tx
	}

	appCache := cache.NewCache(cfg.CacheSize, ".cache.json")

	multimon = multimonpackage.New(cfg.Multimon, captureOutputChan, ps, appCache, txHandle, logger)
	err = multimon.Start()
	if err != nil {
		return nil, fmt.Errorf("Error starting multimon: %v", err)
	}

	if cfg.IGate.Enabled {
		ig, err = igate.New(cfg.IGate, ps, cfg.Transmitter.Enabled, txHandle, cfg.StationCallsign, logger)
		if err != nil {
			return nil, fmt.Errorf("Error creating IGate client: %v", err)
		}
	}

	if cfg.DigipeaterEnabled {
		if txHandle == nil {
			return nil, fmt.Errorf("digipeater enabled but transmitter is disabled")
		}
		dp, err = digipeater.New(txHandle, ps, cfg.StationCallsign, cfg.Digipeater, logger)
		if err != nil {
			return nil, fmt.Errorf("Error creating digipeater: %v", err)
		}
	}

	dg := &DigiGate{
		cfg:               cfg,
		captureOutputChan: captureOutputChan,
		transmitter:       tx,
		stop:              make(chan bool),
		igate:             ig,
		logger:            logger,
		cache:             appCache,
		multimon:          multimon,
		captureDevice:     captureDevice,
		digipeater:        dp,
	}

	return dg, nil
}

func (d *DigiGate) Run() error {
	if d.transmitter != nil {
		err := d.transmitter.Start()
		if err != nil {
			return fmt.Errorf("Error starting transmitter: %v", err)
		}
	}

	go func() {
		for {
			select {
			case <-d.stop:
				d.logger.Info("Stopping capture device")
				d.captureDevice.Stop()

				d.logger.Info("Stopping multimon-ng process")
				d.multimon.Cmd.Process.Kill()

				if d.transmitter != nil {
					d.logger.Info("Stopping transmitter")
					d.transmitter.Stop()
				}

				if d.cfg.IGate.Enabled {
					d.logger.Info("Stopping IGate client")
					d.igate.Stop()
				}

				if d.cfg.DigipeaterEnabled {
					d.logger.Info("Stopping digipeater")
					d.digipeater.Stop()
				}

				return
			}
		}
	}()

	var g errgroup.Group

	if d.cfg.IGate.Enabled {
		g.Go(func() error {
			return d.igate.Run()
		})
	}

	if d.cfg.DigipeaterEnabled {
		g.Go(func() error {
			return d.digipeater.Run()
		})
	}

	return g.Wait()
}

func (d *DigiGate) Stop() {
	d.stop <- true
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
