package monitoring

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
)

const prefix = "slackmcp_"

const (
	MetricLabelTool   = "tool"
	MetricLabelServer = "server"
	MetricLabelError  = "error"

	MetricLabelType  = "type"
	MetricLabelModel = "model"
)

var (
	ToolInvocations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: fmt.Sprintf("%stool_invocations_total", prefix),
			Help: "Total number of tool invocations",
		},
		[]string{MetricLabelTool, MetricLabelServer, MetricLabelError},
	)
	LLMTokensPerRequest = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    fmt.Sprintf("%sllm_tokens", prefix),
			Help:    "Histogram of tokens sent per request to the LLM",
			Buckets: prometheus.ExponentialBuckets(500, 1.5, 20),
		},
		[]string{MetricLabelType, MetricLabelModel},
	)
)

func RegisterMetrics() {
	prometheus.MustRegister(
		ToolInvocations,
		LLMTokensPerRequest,
	)
}
