//go:build windows

package app

import (
	"os"
	"os/signal"
	"syscall"
)

// setupSignalHandlers sets up signal channels and returns cleanup function
// Note: SIGUSR1 is not available on Windows, so reload via signal is not supported
func setupSignalHandlers() (reload, shutdown chan os.Signal, cleanup func()) {
	reloadChan := make(chan os.Signal, 1)
	shutdownChan := make(chan os.Signal, 1)

	// Windows doesn't support SIGUSR1, so reload channel will never receive signals
	// Only setup shutdown signals
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)

	cleanup = func() {
		signal.Stop(reloadChan)
		signal.Stop(shutdownChan)
	}

	return reloadChan, shutdownChan, cleanup
}