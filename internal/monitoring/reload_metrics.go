package monitoring

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	reloadCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_reloads_total",
			Help: "Total number of application reloads",
		},
		[]string{"trigger"}, // "signal", "periodic"
	)

	reloadDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "mcp_reload_duration_seconds",
			Help:    "Time spent performing complete reload cycles",
			Buckets: prometheus.DefBuckets,
		},
	)

	initFailures = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "mcp_init_failures_total",
			Help: "Total number of initialization failures",
		},
	)

	backoffDelay = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "mcp_backoff_delay_seconds",
			Help: "Current exponential backoff delay in seconds",
		},
	)
)

// RecordReload records a successful reload with its duration
func RecordReload(trigger string, duration time.Duration) {
	reloadCounter.WithLabelValues(trigger).Inc()
	reloadDuration.Observe(duration.Seconds())
}

// RecordInitFailure records an initialization failure
func RecordInitFailure() {
	initFailures.Inc()
}

// UpdateBackoffDelay updates the current backoff delay metric
func UpdateBackoffDelay(delay time.Duration) {
	backoffDelay.Set(delay.Seconds())
}
