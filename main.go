//go:build windows

package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"airprint-server/internal/config"
	"airprint-server/internal/ipp"
	"airprint-server/internal/mdns"
	"airprint-server/internal/printer"
)

func main() {
	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("Configuration loaded: printer=%s, port=%d", cfg.PrinterName, cfg.Port)

	// Set up logging level (simplified)
	// TODO: use slog or zap for structured logging

	// Get printer
	winPrinter, err := printer.New("")
	if err != nil {
		log.Fatalf("Failed to connect to printer: %v", err)
	}
	defer winPrinter.Close()
	log.Printf("Connected to printer: %s", winPrinter.Name())

	// Start IPP server
	ippSrv := ipp.NewServer(cfg.Interface, cfg.Port, winPrinter)
	go func() {
		if err := ippSrv.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("IPP server failed: %v", err)
		}
	}()
	defer ippSrv.Stop()

	// Start mDNS advertiser
	mdnsAdv := mdns.NewAdvertiser()
	mdnsTXT := map[string]string{
		"product":   cfg.PrinterModel,
		"note":      cfg.PrinterLocation,
	}
	if err := mdnsAdv.Start(cfg.PrinterName, cfg.Port, mdnsTXT); err != nil {
		log.Fatalf("Failed to start mDNS: %v", err)
	}
	defer mdnsAdv.Stop()

	// Graceful shutdown handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutting down...")
	// Defers will handle cleanup
}
