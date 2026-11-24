package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"net/http"
	"time"
	"syscall"

	"github.com/oorrwullie/sparky/internal/log"
)

func main() {
	logger, err := log.New()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	// Reserve the optional metrics listener before NewSparky can start SDR
	// subprocesses or acquire radio hardware.
	sparky, registry, listener, err := prepareGateway(
		os.Getenv("SPARKY_OBSERVABILITY_ADDR"),
		os.Getenv("SPARKY_OBSERVABILITY_ALLOW_REMOTE")=="1",
		func() (*Sparky,error) {return NewSparky(logger)},
	)
	if err!=nil {logger.Fatal(err)}
	if listener!=nil {
		defer listener.Close()
		server:=&http.Server{
			Handler:registry.Handler(), ReadHeaderTimeout:2*time.Second,
			ReadTimeout:5*time.Second,WriteTimeout:5*time.Second,
			IdleTimeout:15*time.Second,MaxHeaderBytes:4096,
		}
		defer server.Close()
		go func(){
			if err:=server.Serve(listener);err!=nil&&err!=http.ErrServerClosed{
				logger.Error("observability HTTP stopped: ",err)
			}
		}()
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
