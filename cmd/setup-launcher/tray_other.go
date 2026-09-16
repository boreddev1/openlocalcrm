//go:build !windows

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func runTray(ctx context.Context, serverURL string) {
	log.Printf("[Tray] Platform does not require Win32 Tray. Running standard daemon loop.")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-ctx.Done():
	case <-sigChan:
		log.Println("[Tray] Received termination signal. Exiting.")
	}
}
