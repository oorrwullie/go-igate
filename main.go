package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var EnableTx bool

func main() {
	enableTxFlag := flag.Bool("enableTx", false, "Enable TX support")
	flag.Parse()

	if *enableTxFlag {
		EnableTx = true
		fmt.Println("TX support enabled.")
	}

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

		time.Sleep(2 * time.Second)

		os.Exit(0)
	}()

	igate.Logger.Debug("Listening for packets.")

	err = igate.Run()
	if err != nil {
		log.Fatal(err)
	}
}
