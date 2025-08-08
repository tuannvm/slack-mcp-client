package observability

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "time"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
    "github.com/tuannvm/slack-mcp-client/internal/common/logging"
    "github.com/tuannvm/slack-mcp-client/internal/config"
)

// LangfuseProvider provides Langfuse-optimized tracing
type LangfuseProvider struct {
    tracer  trace.Tracer
    logger  *logging.Logger
    config  *config.ObservabilityConfig
    enabled bool
}

// NewLangfuseProvider creates a new Langfuse provider
func NewLangfuseProvider(cfg *config.Config, logger *logging.Logger) *LangfuseProvider {
    return &LangfuseProvider{
        tracer:  otel.Tracer(TracerName),
        logger:  logger,
        config:  &cfg.Observability,
        enabled: cfg.Observability.Enabled,
    }
}

func (p *LangfuseProvider) StartTrace(ctx context.Context, name string, input string, metadata map[string]string) (context.Context, trace.Span) {
    spanCtx, span := p.tracer.Start(ctx, name)

    // Apply Langfuse trace-level attributes
    span.SetAttributes(
        attribute.String("langfuse.trace.name", name),
        attribute.String("langfuse.trace.input", input),
        attribute.String("langfuse.release", p.getServiceVersion()),
        attribute.String("langfuse.environment", p.getEnvironment()),
        attribute.Bool("langfuse.trace.public", false),

        // Standard attributes
        attribute.String("service.name", p.getServiceName()),
        attribute.String("service.version", p.getServiceVersion()),
        attribute.String("input.value", input),
        attribute.Int("input.length", len(input)),
    )

    // Add metadata with Langfuse prefix for queryability
    for key, value := range metadata {
        span.SetAttributes(
            attribute.String(fmt.Sprintf("langfuse.trace.metadata.%s", key), value),
            attribute.String(key, value), // Also add as regular attribute
        )
    }

    return spanCtx, span
}

func (p *LangfuseProvider) StartSpan(ctx context.Context, name string, spanType string, input string, metadata map[string]string) (context.Context, trace.Span) {
    spanCtx, span := p.tracer.Start(ctx, name)

    // Apply Langfuse observation-level attributes
    if spanType == "" {
        spanType = "span"
    }

    span.SetAttributes(
        attribute.String("langfuse.observation.type", spanType),
        attribute.String("langfuse.observation.level", "DEFAULT"),
    )

    if input != "" {
        span.SetAttributes(
            attribute.String("langfuse.observation.input", input),
            attribute.String("input.value", input),
            attribute.Int("input.length", len(input)),
        )
    }

    // Add metadata with Langfuse prefix
    for key, value := range metadata {
        span.SetAttributes(
            attribute.String(fmt.Sprintf("langfuse.observation.metadata.%s", key), value),
            attribute.String(key, value), // Also add as regular attribute
        )
    }

    return spanCtx, span
}

func (p *LangfuseProvider) StartLLMSpan(ctx context.Context, name string, model string, input string, parameters map[string]interface{}) (context.Context, trace.Span) {
    spanCtx, span := p.tracer.Start(ctx, name)

    // Apply Langfuse LLM generation attributes
    span.SetAttributes(
        attribute.String("langfuse.observation.type", "generation"),
        attribute.String("langfuse.observation.level", "DEFAULT"),
        attribute.String("langfuse.observation.model.name", model),
        attribute.String("langfuse.observation.input", input),

        // Standard GenAI semantic conventions
        // attribute.String("gen_ai.request.model", model),
        // attribute.String("gen_ai.prompt", input),

        // Standard attributes
        // attribute.String("model", model),
        // attribute.String("llm.model_name", model),
        // attribute.String("input.value", input),
        // attribute.Int("input.length", len(input)),
    )
	// Add model parameters in Langfuse format
    if len(parameters) > 0 {
        if paramsJSON, err := json.Marshal(parameters); err == nil {
            span.SetAttributes(
                attribute.String("langfuse.observation.model.parameters", string(paramsJSON)),
            )
        }

        // Also add as individual gen_ai attributes
        // for key, value := range parameters {
        //     switch v := value.(type) {
        //     case string:
        //         span.SetAttributes(attribute.String(fmt.Sprintf("gen_ai.request.%s", key), v))
        //     case int:
        //         span.SetAttributes(attribute.Int(fmt.Sprintf("gen_ai.request.%s", key), v))
        //     case float64:
        //         span.SetAttributes(attribute.Float64(fmt.Sprintf("gen_ai.request.%s", key), v))
        //     case bool:
        //         span.SetAttributes(attribute.Bool(fmt.Sprintf("gen_ai.request.%s", key), v))
        //     }
        // }
    }

    return spanCtx, span
}

func (p *LangfuseProvider) SetOutput(span trace.Span, output string) {
    span.SetAttributes(
        attribute.String("langfuse.observation.output", output),
        attribute.String("langfuse.trace.output", output), // Will be overridden by child spans
        // attribute.String("gen_ai.completion", output),
        // attribute.String("output.value", output),
        // attribute.Int("output.length", len(output)),
    )
}

func (p *LangfuseProvider) SetTokenUsage(span trace.Span, promptTokens, completionTokens, totalTokens int) {
    // Langfuse usage format
    usageDetails := map[string]int{
        "prompt_tokens":     promptTokens,
        "completion_tokens": completionTokens,
        "total_tokens":      totalTokens,
    }

    if usageJSON, err := json.Marshal(usageDetails); err == nil {
        span.SetAttributes(
            attribute.String("langfuse.observation.usage_details", string(usageJSON)),
        )
    }

    // Standard GenAI semantic conventions
    // span.SetAttributes(
    //     attribute.Int("gen_ai.usage.prompt_tokens", promptTokens),
    //     attribute.Int("gen_ai.usage.completion_tokens", completionTokens),
    //     attribute.Int("gen_ai.usage.total_tokens", totalTokens),

        // Additional standard formats
        // attribute.Int("llm.token_count.prompt", promptTokens),
        // attribute.Int("llm.token_count.completion", completionTokens),
        // attribute.Int("llm.token_count.total", totalTokens),
        // attribute.Int("tokens.prompt", promptTokens),
        // attribute.Int("tokens.completion", completionTokens),
        // attribute.Int("tokens.total", totalTokens),
    // )
}

func (p *LangfuseProvider) SetDuration(span trace.Span, duration time.Duration) {
    span.SetAttributes(
        attribute.Float64("duration.seconds", duration.Seconds()),
        attribute.Int64("duration.milliseconds", duration.Milliseconds()),
    )
}

func (p *LangfuseProvider) RecordError(span trace.Span, err error, level string) {
    if err == nil {
        return
    }

    if level == "" {
        level = "ERROR"
    }

    // Langfuse error format
    span.SetAttributes(
        attribute.String("langfuse.observation.level", level),
        attribute.String("langfuse.observation.status_message", err.Error()),
    )

    span.RecordError(err)
    span.SetStatus(codes.Error, err.Error())
}

func (p *LangfuseProvider) RecordSuccess(span trace.Span, message string) {
    span.SetAttributes(
        attribute.String("langfuse.observation.level", "DEFAULT"),
		attribute.String("langfuse.observation.status_message", message),
    )
    span.SetStatus(codes.Ok, message)
}

func (p *LangfuseProvider) GetProviderType() TracingProvider {
    return ProviderLangfuse
}

func (p *LangfuseProvider) IsEnabled() bool {
    return p.enabled
}

// Helper methods
func (p *LangfuseProvider) getServiceName() string {
    if p.config != nil && p.config.ServiceName != "" {
        return p.config.ServiceName
    }
    return "slack-mcp-client"
}

func (p *LangfuseProvider) getServiceVersion() string {
    if p.config != nil && p.config.ServiceVersion != "" {
        return p.config.ServiceVersion
    }
    return "1.0.0"
}

func (p *LangfuseProvider) getEnvironment() string {
    if env := os.Getenv("ENVIRONMENT"); env != "" {
        return env
    }
    if env := os.Getenv("ENV"); env != "" {
        return env
    }
    return "development"
}