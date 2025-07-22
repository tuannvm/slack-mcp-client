//go:build unix

package app

import (
	"os"
	"os/signal"
	"syscall"
)

// setupSignalHandlers sets up signal channels and returns cleanup function
func setupSignalHandlers() (reload, shutdown chan os.Signal, cleanup func()) {
	reloadChan := make(chan os.Signal, 1)
	shutdownChan := make(chan os.Signal, 1)

	signal.Notify(reloadChan, syscall.SIGUSR1)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)

	cleanup = func() {
		signal.Stop(reloadChan)
		signal.Stop(shutdownChan)
	}

	return reloadChan, shutdownChan, cleanup
}