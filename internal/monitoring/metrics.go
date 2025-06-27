package monitoring

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
)

const prefix = "slackmcp_"

var (
	ToolInvocations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: fmt.Sprintf("%stool_invocations_total", prefix),
			Help: "Total number of tool invocations",
		},
		[]string{"tool", "server", "error"},
	)
	LLMTokensPerRequest = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    fmt.Sprintf("%sllm_tokens", prefix),
			Help:    "Histogram of tokens sent per request to the LLM",
			Buckets: prometheus.ExponentialBuckets(10, 2, 10), // 10, 20, 40, ..., 5120
		},
	)
)

func RegisterMetrics() {
	prometheus.MustRegister(
		ToolInvocations,
		LLMTokensPerRequest,
	)
}
