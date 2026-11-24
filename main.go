package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"net"
	"net/http"
	"time"
	"strings"
	"syscall"

	"github.com/oorrwullie/sparky/internal/log"
	"github.com/oorrwullie/sparky/internal/observability"
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

	// A disabled-by-default listener is owned by the gateway process, not the
	// separate administrative config UI. It never accesses radio hardware.
	var o11yServer *http.Server
	if addr:=strings.TrimSpace(os.Getenv("SPARKY_OBSERVABILITY_ADDR"));addr!="" {
		if _,_,err:=net.SplitHostPort(addr);err!=nil{
			logger.Fatal("invalid SPARKY_OBSERVABILITY_ADDR: ",err)
		}
		registry:=observability.New()
		sparky.SetObservability(registry)
		listener,err:=net.Listen("tcp",addr)
		if err!=nil{logger.Fatal("observability listener: ",err)}
		o11yServer=&http.Server{
			Handler:registry.Handler(), ReadHeaderTimeout:2*time.Second,
			ReadTimeout:5*time.Second,WriteTimeout:5*time.Second,
			IdleTimeout:15*time.Second,MaxHeaderBytes:4096,
		}
		go func(){
			if err:=o11yServer.Serve(listener);err!=nil && err!=http.ErrServerClosed{
				logger.Error("observability HTTP stopped: ",err)
			}
		}()
		defer o11yServer.Close()
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
