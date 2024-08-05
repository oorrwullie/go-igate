package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oorrwullie/go-igate/internal/log"
)

func main() {
	logger, err := log.New()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	digiGate, err := NewDigiGate(logger)
	if err != nil {
		logger.Fatal(err.Error())
	}

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan,
		syscall.SIGINT,
		syscall.SIGKILL,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	// Listen for exit signals, set up for a clean exit.
	go func() {
		signal := <-sigchan
		logger.Debug(fmt.Sprintf("Signal (%s) caught, terminating processes.", signal))

		digiGate.Stop()
		time.Sleep(2 * time.Second)

		os.Exit(0)
	}()

	logger.Debug("Listening for packets.")

	err = digiGate.Run()
	if err != nil {
		logger.Fatal(err)
	}
}
