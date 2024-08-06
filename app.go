package main

import (
	"fmt"

	"github.com/oorrwullie/go-igate/internal/cache"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/digipeater"
	"github.com/oorrwullie/go-igate/internal/igate"
	"github.com/oorrwullie/go-igate/internal/log"
	multimonpackage "github.com/oorrwullie/go-igate/internal/multimon"
	sdrpackage "github.com/oorrwullie/go-igate/internal/sdr"
	"github.com/oorrwullie/go-igate/internal/transmitter"

	"golang.org/x/sync/errgroup"
)

type (
	DigiGate struct {
		cfg                config.Config
		sdr                *sdrpackage.Sdr
		sdrOutputChan      chan []byte
		multimon           *multimonpackage.Multimon
		multimonOutputChan chan string
		transmitter        *transmitter.Transmitter
		stop               chan bool
		igate              *igate.IGate
		digipeater         *digipeater.Digipeater
		logger             *log.Logger
		cache              *cache.Cache
	}
)

const minPacketSize = 35

func NewDigiGate(logger *log.Logger) (*DigiGate, error) {
	var (
		tx                 *transmitter.Transmitter
		ig                 *igate.IGate
		dp                 *digipeater.Digipeater
		sdr                *sdrpackage.Sdr
		sdrOutputChan      = make(chan []byte)
		multimon           *multimonpackage.Multimon
		multimonOutputChan = make(chan string)
	)

	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	sdr = sdrpackage.New(cfg.Sdr, sdrOutputChan, logger)
	err = sdr.Start()
	if err != nil {
		return nil, fmt.Errorf("Error starting SDR: %v", err)
	}

	appCache := cache.NewCache(cfg.CacheSize, ".cache.json")

	multimon = multimonpackage.New(cfg.Multimon, sdrOutputChan, multimonOutputChan, appCache, logger)
	err = multimon.Start()
	if err != nil {
		return nil, fmt.Errorf("Error starting multimon: %v", err)
	}

	if cfg.Transmitter.Enabled {
		tx, err = transmitter.New(cfg.Transmitter, logger)
		if err != nil {
			return nil, fmt.Errorf("Error creating transmitter: %v", err)
		}
	}

	if cfg.IGate.Enabled {
		ig, err = igate.New(cfg.IGate, multimonOutputChan, cfg.Transmitter.Enabled, tx.TxChan, cfg.StationCallsign, logger)
		if err != nil {
			return nil, fmt.Errorf("Error creating IGate client: %v", err)
		}
	}

	if cfg.DigipeaterEnabled {
		dp = digipeater.New(tx.TxChan, cfg.StationCallsign, logger)
	}

	dg := &DigiGate{
		cfg:                cfg,
		sdrOutputChan:      sdrOutputChan,
		multimonOutputChan: multimonOutputChan,
		transmitter:        tx,
		stop:               make(chan bool),
		igate:              ig,
		logger:             logger,
		cache:              appCache,
		multimon:           multimon,
		sdr:                sdr,
		digipeater:         dp,
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
				d.logger.Info("Stopping rtl_fm process")
				d.sdr.Cmd.Process.Kill()

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
