package main

import (
	"flag"
	"fmt"
	"github.com/oorrwullie/go-igate/internal/transmitter"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oorrwullie/go-igate/internal/aprs"
	"github.com/oorrwullie/go-igate/internal/config"
	"github.com/oorrwullie/go-igate/internal/igate"
	"github.com/oorrwullie/go-igate/internal/log"
)

func main() {
	var tx *transmitter.Transmitter
	enableTxFlag := flag.Bool("enableTx", false, "Enable TX support")
	flag.Parse()

	logger, err := log.New()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	if *enableTxFlag {
		tx = setupTransmitter(logger)
		fmt.Println("TX support enabled.")
	}

	igate, err := setupIGate(tx, logger)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	// Listen for exit signals, set up for a clean exit.
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan,
		syscall.SIGINT,
		syscall.SIGKILL,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		signal := <-sigchan
		logger.Debug(fmt.Sprintf("Signal (%s) caught, terminating processes.", signal))

		igate.Stop <- true
		igate.Aprsis.Disconnect()

		time.Sleep(2 * time.Second)

		os.Exit(0)
	}()

	igate.Logger.Debug("Listening for packets.")

	err = igate.Run()
	if err != nil {
		logger.Fatal(err)
	}
}

func setupIGate(tx *transmitter.Transmitter, logger *log.Logger) (*igate.IGate, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	aprsis, err := aprs.NewAprsIs(cfg.AprsIs, logger)
	if err != nil {
		return nil, err
	}

	ig, err := igate.New(cfg, aprsis, tx, logger)
	if err != nil {
		return nil, err
	}

	return ig, nil

}

func setupTransmitter(logger *log.Logger) *transmitter.Transmitter {
	t := &transmitter.Transmitter{
		TxChan: make(chan string),
		Stop:   make(chan bool),
		Logger: logger,
	}

	err := t.StartTx()
	if err != nil {
		logger.Error("Error starting transmitter: ", err)
	}

	return t
}
