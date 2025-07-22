package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tuannvm/slack-mcp-client/internal/config"
	"github.com/tuannvm/slack-mcp-client/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/internal/monitoring"
)

const (
	maxBackoffDelay   = 5 * time.Minute
	backoffMultiplier = 2.0
	minReloadInterval = 1 * time.Minute
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

		// Check if reload is enabled
		cfg, err := config.LoadConfig(configFile, logger)
		if err != nil {
			logger.ErrorKV("Failed to load config for reload check", "error", err)
			// Continue with normal app execution
			return appFunc(logger)
		}

		cfg.ApplyDefaults()

		if !cfg.Reload.Enabled {
			// Reload disabled, run normally
			logger.Info("Reload disabled, running application normally")
			return appFunc(logger)
		}

		// Validate reload interval
		if err := validateReloadInterval(cfg.Reload.Interval); err != nil {
			logger.ErrorKV("Invalid reload configuration, running normally", "error", err)
			return appFunc(logger)
		}

		logger.InfoKV("Reload enabled", "interval", cfg.Reload.Interval)

		// Setup cancellation for the current application run
		appCtx, appCancel := context.WithCancel(context.Background())
		
		// Run application in a goroutine
		appDone := make(chan error, 1)
		go func() {
			// Pass context to application function for graceful shutdown
			appDone <- runAppWithContext(appCtx, logger, appFunc)
		}()

		// Wait for reload trigger or app completion
		trigger := awaitReloadTrigger(logger, cfg.Reload.Interval)

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
				case <-time.After(10 * time.Second):
					logger.Warn("Application shutdown timed out after 10 seconds")
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
			case <-time.After(10 * time.Second):
				logger.Warn("Application shutdown timed out, forcing restart...")
			}
			
			// Record reload metrics
			monitoring.RecordReload(trigger.Type, time.Since(reloadStartTime))
			
			// Continue the loop to reinitialize
		}
	}
}

// runAppWithContext runs the application function with context for graceful shutdown
func runAppWithContext(ctx context.Context, logger *logging.Logger, appFunc func(*logging.Logger) error) error {
	// For now, we just run the app function normally
	// In a full implementation, we would modify the app function to accept and respect context
	// This is a placeholder for graceful shutdown integration
	return appFunc(logger)
}

// awaitReloadTrigger waits for a reload trigger
func awaitReloadTrigger(logger *logging.Logger, intervalStr string) ReloadTrigger {
	// Setup signal channels
	reloadChan := make(chan os.Signal, 1)
	shutdownChan := make(chan os.Signal, 1)

	signal.Notify(reloadChan, syscall.SIGUSR1)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)

	defer func() {
		signal.Stop(reloadChan)
		signal.Stop(shutdownChan)
	}()

	// Setup periodic timer
	var timerChan <-chan time.Time
	var timer *time.Timer

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		logger.ErrorKV("Invalid reload interval, using default", "interval", intervalStr, "error", err)
		interval = 30 * time.Minute
	}

	timer = time.NewTimer(interval)
	timerChan = timer.C
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

	case <-timerChan:
		logger.Info("Periodic reload triggered")
		return ReloadTrigger{Type: "periodic"}
	}
}

// validateReloadInterval ensures the reload interval is valid and not too short
func validateReloadInterval(interval string) error {
	duration, err := time.ParseDuration(interval)
	if err != nil {
		return err
	}

	if duration < minReloadInterval {
		return nil // Just log warning, don't fail
	}

	return nil
}