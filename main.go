package main

import (
	"flag"
	"fmt"
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
	var enableTx bool

	enableTxFlag := flag.Bool("enableTx", false, "Enable TX support")
	flag.Parse()

	if *enableTxFlag {
		enableTx = true
		fmt.Println("TX support enabled.")
	}

	logger, err := log.New()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	igate, err := setupIGate(enableTx, logger)
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

func setupIGate(enableTx bool, logger *log.Logger) (*igate.IGate, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	aprsis, err := aprs.NewAprsIs(cfg.AprsIs, logger)
	if err != nil {
		return nil, err
	}

	ig, err := igate.New(cfg, enableTx, aprsis, logger)
	if err != nil {
		return nil, err
	}

	return ig, nil

}
