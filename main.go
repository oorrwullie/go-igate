package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	igate, err := NewIGate()
	if err != nil {
		log.Fatal(err)
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
		igate.Logger.Debug(fmt.Sprintf("Signal (%s) caught, terminating processes.", signal))

		igate.Stop <- true
		igate.Aprsis.Disconnect()

		os.Exit(0)
	}()

	igate.Logger.Debug("Listening for packets.")

	err = igate.Run()
	if err != nil {
		log.Fatal(err)
	}
}
