package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oorrwullie/sparky/internal/log"
)

func main() {
	logger, err := log.New()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	sparky, err := NewSparky(logger)
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

		sparky.Stop()
		time.Sleep(2 * time.Second)

		os.Exit(0)
	}()

	logger.Debug("Listening for packets.")

	err = sparky.Run()
	if err != nil {
		logger.Fatal(err)
	}
}
