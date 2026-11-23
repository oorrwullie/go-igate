package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

	ctx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stopSignals()
	go func() {
		<-ctx.Done()
		logger.Info("Shutdown signal received")
		sparky.Stop()
	}()

	logger.Debug("Listening for packets.")

	err = sparky.Run()
	if err != nil {
		logger.Fatal(err)
	}
}
