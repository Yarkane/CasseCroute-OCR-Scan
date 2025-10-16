package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
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

	// Goroutine to listen for OS signals and watcher triggers.
	wg.Add(1)
	go func() {
		defer wg.Done()

		abort := make(chan os.Signal, 1)
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

	// Start a periodic cleaner that removes files older than 24 hours from
	// both input and output directories. Runs every hour.
	const maxAge = 3 * time.Hour
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		// perform an initial cleanup immediately
		cleanupDirs := func() {
			cutoff := time.Now().Add(-maxAge)
			dirs := []string{config.InputDir, config.OutputDir}
			for _, d := range dirs {
				entries, err := os.ReadDir(d)
				if err != nil {
					logger.Debugf("cleanup: cannot read dir %s: %v", d, err)
					continue
				}
				for _, e := range entries {
					if e.IsDir() {
						continue
					}
					fi, err := e.Info()
					if err != nil {
						continue
					}
					if fi.ModTime().Before(cutoff) {
						p := filepath.Join(d, e.Name())
						if err := os.Remove(p); err != nil {
							logger.Debugf("cleanup: failed to remove %s: %v", p, err)
						} else {
							logger.Infof("cleanup: removed old file %s", p)
						}
					}
				}
			}
		}

		cleanupDirs()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanupDirs()
			}
		}
	}()

	// Wait for cancellation and then gracefully shutdown web server
	go func() {
		<-ctx.Done()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelShutdown()
		if err := webSrv.Shutdown(shutdownCtx); err != nil {
			logger.Errorf("Error shutting down web server: %v", err)
		}
	}()

	wg.Wait()
	logger.Info("All done. Exiting.")
}
