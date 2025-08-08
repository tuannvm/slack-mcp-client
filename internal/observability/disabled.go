package observability

import (
    "context"
    "time"

    "go.opentelemetry.io/otel/trace"
    "github.com/tuannvm/slack-mcp-client/internal/common/logging"
)

// DisabledProvider provides no-op tracing when observability is disabled
type DisabledProvider struct {
    logger *logging.Logger
}

// NewDisabledProvider creates a new disabled provider
func NewDisabledProvider(logger *logging.Logger) *DisabledProvider {
    return &DisabledProvider{
        logger: logger,
    }
}

// No-op implementations
func (p *DisabledProvider) StartTrace(ctx context.Context, name string, input string, metadata map[string]string) (context.Context, trace.Span) {
    return ctx, trace.SpanFromContext(ctx)
}

func (p *DisabledProvider) StartSpan(ctx context.Context, name string, spanType string, input string, metadata map[string]string) (context.Context, trace.Span) {
    return ctx, trace.SpanFromContext(ctx)
}

func (p *DisabledProvider) StartLLMSpan(ctx context.Context, name string, model string, input string, parameters map[string]interface{}) (context.Context, trace.Span) {
    return ctx, trace.SpanFromContext(ctx)
}

func (p *DisabledProvider) SetOutput(span trace.Span, output string) {
    // No-op
}

func (p *DisabledProvider) SetTokenUsage(span trace.Span, promptTokens, completionTokens, totalTokens int) {
    // No-op
}

func (p *DisabledProvider) SetDuration(span trace.Span, duration time.Duration) {
    // No-op
}

func (p *DisabledProvider) RecordError(span trace.Span, err error, level string) {
    // No-op
}

func (p *DisabledProvider) RecordSuccess(span trace.Span, message string) {
    // No-op
}

func (p *DisabledProvider) GetProviderType() TracingProvider {
    return ProviderDisabled
}

func (p *DisabledProvider) IsEnabled() bool {
    return false
}