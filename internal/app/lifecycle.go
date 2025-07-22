package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/monitoring"
)

const (
	minReloadInterval      = 10 * time.Second
	defaultShutdownTimeout = 10 * time.Second
)

// ReloadTrigger represents the type of trigger that caused a reload
type ReloadTrigger struct {
	Type   string // "signal", "periodic", "shutdown"
	Signal os.Signal
}

// RunWithReload wraps the main application function with reload capability
func RunWithReload(logger *logging.Logger, configFile string, appFunc func(*logging.Logger) error) error {
	for {
		reloadStartTime := time.Now()

		// Load and validate configuration
		reloadInterval, shouldReload, err := loadAndValidateReloadConfig(configFile, logger)
		if err != nil || !shouldReload {
			// Either config loading failed or reload is disabled - run normally
			return appFunc(logger)
		}

		logger.InfoKV("Reload enabled", "interval", reloadInterval)

		// Setup cancellation for the current application run
		appCtx, appCancel := context.WithCancel(context.Background())

		// Run application in a goroutine
		appDone := make(chan error, 1)
		go func() {
			// Pass context to application function for graceful shutdown
			appDone <- runAppWithContext(appCtx, logger, appFunc)
		}()

		// Wait for reload trigger or app completion
		trigger := awaitReloadTrigger(logger, reloadInterval)

		// Handle the trigger
		select {
		case err := <-appDone:
			// App completed normally before any reload trigger
			logger.InfoKV("Application completed", "error", err)
			appCancel()
			return err

		default:
			// App is still running, we got a reload trigger
			if trigger.Type == "shutdown" {
				logger.InfoKV("Shutdown triggered, gracefully stopping...", "signal", trigger.Signal)
				appCancel() // Signal app to shutdown

				// Wait for app to finish shutting down
				select {
				case <-appDone:
					logger.Info("Application shutdown completed")
				case <-time.After(defaultShutdownTimeout):
					logger.WarnKV("Application shutdown timed out", "timeout", defaultShutdownTimeout)
				}
				return nil
			}

			logger.InfoKV("Reload triggered, shutting down current instance...", "type", trigger.Type)

			// Cancel current application
			appCancel()

			// Wait for current app to shutdown gracefully
			select {
			case <-appDone:
				logger.Info("Current application instance shut down, reinitializing...")
			case <-time.After(defaultShutdownTimeout):
				logger.WarnKV("Application shutdown timed out, forcing restart", "timeout", defaultShutdownTimeout)
			}

			// Record reload metrics
			monitoring.RecordReload(trigger.Type, time.Since(reloadStartTime))

			// Continue the loop to reinitialize
		}
	}
}

// runAppWithContext runs the application function with context for graceful shutdown
// Note: The context is available but not yet fully integrated with the application.
// Future enhancement: Modify application functions to accept context for proper cancellation.
func runAppWithContext(ctx context.Context, logger *logging.Logger, appFunc func(*logging.Logger) error) error {
	// Run the application function
	// The context could be used in the future for cancellation signals
	return appFunc(logger)
}

// awaitReloadTrigger waits for a reload trigger
func awaitReloadTrigger(logger *logging.Logger, interval time.Duration) ReloadTrigger {
	// Setup signal channels with proper cleanup
	reloadChan, shutdownChan, cleanup := setupSignalHandlers()
	defer cleanup()

	// Setup periodic timer
	timer := time.NewTimer(interval)
	defer timer.Stop()

	logger.InfoKV("Waiting for reload trigger", "interval", interval)

	// Wait for trigger
	select {
	case sig := <-reloadChan:
		logger.InfoKV("Reload signal received", "signal", sig)
		return ReloadTrigger{Type: "signal", Signal: sig}

	case sig := <-shutdownChan:
		logger.InfoKV("Shutdown signal received", "signal", sig)
		return ReloadTrigger{Type: "shutdown", Signal: sig}

	case <-timer.C:
		logger.Info("Periodic reload triggered")
		return ReloadTrigger{Type: "periodic"}
	}
}


// validateReloadInterval ensures the reload interval is valid and not too short
func validateReloadInterval(interval string) error {
	duration, err := time.ParseDuration(interval)
	if err != nil {
		return fmt.Errorf("invalid duration format: %w", err)
	}

	if duration < minReloadInterval {
		return fmt.Errorf("reload interval %s is below minimum of %s", duration, minReloadInterval)
	}

	return nil
}

// loadAndValidateReloadConfig loads configuration and validates reload settings
// Returns: (reloadInterval, shouldReload, error)
func loadAndValidateReloadConfig(configFile string, logger *logging.Logger) (time.Duration, bool, error) {
	// Load configuration
	cfg, err := config.LoadConfig(configFile, logger)
	if err != nil {
		logger.ErrorKV("Failed to load config for reload check", "error", err)
		return 0, false, err
	}

	cfg.ApplyDefaults()

	// Check if reload is enabled
	if !cfg.Reload.Enabled {
		logger.Info("Reload disabled, running application normally")
		return 0, false, nil
	}

	// Validate reload interval
	if err := validateReloadInterval(cfg.Reload.Interval); err != nil {
		logger.ErrorKV("Invalid reload configuration, running normally", "error", err)
		return 0, false, err
	}

	// Parse interval
	reloadInterval, err := time.ParseDuration(cfg.Reload.Interval)
	if err != nil {
		// This shouldn't happen after validation, but handle gracefully
		logger.ErrorKV("Failed to parse reload interval after validation", "error", err)
		return 0, false, err
	}

	return reloadInterval, true, nil
}
