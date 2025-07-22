# Signal-Based Reload Implementation (Implemented)

This document outlines the implemented zero-downtime approach to address [GitHub Issue #56](https://github.com/tuannvm/slack-mcp-client/issues/56) using signal-based application reload.

## Problem Statement

When MCP servers restart in Kubernetes deployments, the Slack MCP client needs to reload to reconnect and discover tools, but should avoid downtime.

## Solution Overview

Implemented **signal-based reload** using SIGUSR1 and periodic timers that trigger a complete application reload within the same process, ensuring zero downtime.

## Advantages

- **Zero downtime**: Continuous operation during reload
- **On-demand reload**: `kubectl exec pod -- kill -USR1 1`
- **Faster**: No Kubernetes restart delay (~10s saved)
- **Fresh state**: Complete reinitialization of all components
- **Production-ready**: Comprehensive error handling and resource cleanup

## Implementation

### Configuration Structure
```go
type ReloadConfig struct {
    Enabled  bool   `json:"enabled,omitempty"`  // Enable periodic reload (default: false)
    Interval string `json:"interval,omitempty"` // Reload interval (default: "30m")
}
```

### Core Components

1. **Application Lifecycle** (`internal/app/lifecycle.go`)
   - `RunWithReload()` - Main wrapper function
   - Signal handling for SIGUSR1 (reload) and SIGINT/SIGTERM (shutdown)
   - Periodic timer for automatic reloads
   - Graceful shutdown with 10-second timeout

2. **Configuration Integration** (`internal/config/config.go`)
   - Added ReloadConfig to main Config struct
   - Default values: disabled, 30-minute interval
   - Minimum interval validation (10 seconds)

3. **Monitoring Metrics** (`internal/monitoring/reload_metrics.go`)
   - Reload counters by trigger type (signal, periodic)
   - Reload duration histogram
   - Prometheus metrics endpoint

### Key Features

- **Centralized timeouts**: Constants for shutdown and minimum intervals
- **Helper functions**: Configuration loading, signal handling setup
- **Structured logging**: Key-value pairs for better observability
- **Error handling**: Graceful fallback to normal operation on config errors
- **Resource cleanup**: Proper signal handler cleanup

## Configuration Examples

### Enable with custom interval
```json
{
  "reload": {
    "enabled": true,
    "interval": "15m"
  }
}
```

### Disabled (Default)
```json
{
  "reload": {
    "enabled": false
  }
}
```

## Usage

### On-Demand Reload
```bash
# In Kubernetes
kubectl exec -it <pod-name> -- kill -USR1 1

# Local process
kill -USR1 <process-id>
```

### Periodic Reload
- Automatically reloads based on configured interval
- Minimum interval: 10 seconds
- Default interval: 30 minutes

## Testing

The implementation includes comprehensive unit tests:
- Signal handling validation
- Configuration parsing and validation
- Timeout constant verification
- Trigger type handling

## Benefits

1. **Zero Downtime**: Application continues running during reload
2. **Flexible**: Both manual (signal) and automatic (periodic) triggers
3. **Safe**: Minimum interval prevents excessive reloading
4. **Observable**: Prometheus metrics for monitoring
5. **Maintainable**: Clean, modular code with helper functions

## Production Deployment

Works seamlessly with Kubernetes:
- Pod stays running during reloads
- No service interruption
- Compatible with health checks
- Metrics available for monitoring dashboards

## Monitoring

Available Prometheus metrics:
- `mcp_reloads_total` - Counter by trigger type
- `mcp_reload_duration_seconds` - Reload timing histogram

## Implementation Status

✅ **Complete** - Fully implemented and tested
- Configuration structure and validation
- Signal-based and periodic reload triggers  
- Graceful shutdown handling
- Prometheus metrics integration
- Comprehensive unit test coverage