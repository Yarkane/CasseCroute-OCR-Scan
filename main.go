package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/xperimental/autoocr/processor"
	"github.com/xperimental/autoocr/server"
	"github.com/xperimental/autoocr/watcher"
)

func main() {
	config, err := parseArgs()
	if err != nil {
		log.Fatalf("Error parsing arguments: %s", err)
	}

	logger := config.CreateLogger()

	logger.Debugf("Input: %s", config.InputDir)
	logger.Debugf("Output: %s", config.OutputDir)
	logger.Debugf("Permissions: %s", os.FileMode(config.OutPermissions))

	wg := &sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())

	watcher, err := watcher.New(ctx, logger, config.InputDir, config.Delay)
	if err != nil {
		logger.Fatalf("Error creating watcher: %s", err)
	}
	watcher.Start(wg)

	// Start minimal web server to accept uploads
	webSrv := server.New(":8080", config.InputDir, config.OutputDir, logger)
	if err := webSrv.Start(wg); err != nil {
		logger.Fatalf("Error starting web server: %s", err)
	}

	processor, err := processor.New(ctx, logger, config.ProcessorConfig())
	if err != nil {
		logger.Fatalf("Error creating processor: %s", err)
	}
	processor.Start(wg)

	go func() {
		wg.Add(1)
		defer wg.Done()

		abort := make(chan os.Signal)
		signal.Notify(abort, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(abort)

		logger.Info("Waiting for changes...")
		for {
			select {
			case <-abort:
				cancel()
				return
			case <-watcher.Trigger:
				processor.Trigger()
			}
		}
	}()

	// Wait for cancellation and then gracefully shutdown web server
	go func() {
		<-ctx.Done()
		shutdownCtx, _ := context.WithTimeout(context.Background(), 5*time.Second)
		if err := webSrv.Shutdown(shutdownCtx); err != nil {
			logger.Errorf("Error shutting down web server: %v", err)
		}
	}()

	wg.Wait()
	logger.Info("All done. Exiting.")
}
