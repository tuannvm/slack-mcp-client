package observability

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/tuannvm/slack-mcp-client/v2/internal/common/logging"
	"github.com/tuannvm/slack-mcp-client/v2/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	OtelTrace "go.opentelemetry.io/otel/trace"
)

type LangfuseProvider struct {
	tracer         OtelTrace.Tracer
	logger         *logging.Logger
	config         *config.ObservabilityConfig
	tracerProvider *trace.TracerProvider
	cleanup        func()
	enabled        bool
}

// NewLangfuseProvider creates a new Langfuse provider
func NewLangfuseProvider(cfg *config.Config, logger *logging.Logger) *LangfuseProvider {
	provider := &LangfuseProvider{
		logger:  logger,
		config:  &cfg.Observability,
		enabled: false,
	}

	// Setup OpenTelemetry SDK for Langfuse
	cleanup := provider.setupOpenTelemetry()
	provider.cleanup = cleanup

	if provider.tracerProvider != nil {
		provider.tracer = otel.Tracer(TracerName)
		provider.enabled = true
		logger.Info("Langfuse provider initialized successfully")
	} else {
		logger.Warn("Langfuse provider initialization failed")
	}

	return provider
}

// setupOpenTelemetry configures OpenTelemetry for Langfuse (your original logic)
func (p *LangfuseProvider) setupOpenTelemetry() func() {
	ctx := context.Background()

	// Get credentials from config
	endpoint := p.config.Endpoint
	publicKey := p.config.PublicKey
	secretKey := p.config.SecretKey

	if endpoint == "" || publicKey == "" || secretKey == "" {
		p.logger.Warn("Langfuse tracing disabled: endpoint, publicKey, or secretKey not set")
		return func() {} // No-op cleanup
	}

	// Create Basic Auth string: base64(publicKey:secretKey)
	authString := fmt.Sprintf("%s:%s", publicKey, secretKey)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(authString))

	// Create OTLP trace exporter
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": fmt.Sprintf("Basic %s", encodedAuth),
		}),
	)
	if err != nil {
		p.logger.ErrorKV("Failed to create OTLP trace exporter", "error", err)
		return func() {}
	}
	// Create tracer provider
	p.tracerProvider = trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(resource.NewWithAttributes("",
			attribute.String("service.name", p.getServiceName()),
			attribute.String("service.version", p.getServiceVersion()),
		)),
	)

	otel.SetTracerProvider(p.tracerProvider)
	p.logger.InfoKV("Langfuse OpenTelemetry initialized", "endpoint", endpoint)

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := p.tracerProvider.Shutdown(shutdownCtx); err != nil {
			p.logger.ErrorKV("Error shutting down tracer provider", "error", err)
		} else {
			p.logger.Info("Langfuse provider shutdown successfully")
		}
	}
}

func (p *LangfuseProvider) StartTrace(ctx context.Context, name string, input string, metadata map[string]string) (context.Context, OtelTrace.Span) {
	if p.tracer == nil {
		return ctx, OtelTrace.SpanFromContext(ctx)
	}
	spanCtx, span := p.tracer.Start(ctx, name)

	// Apply Langfuse trace-level attributes
	span.SetAttributes(
		attribute.String("langfuse.trace.name", name),
		attribute.String("langfuse.user.id", metadata["user_email"]),
		attribute.String("langfuse.session.id", metadata["session_id"]),
		attribute.String("langfuse.trace.input", input),
		attribute.String("langfuse.release", p.getServiceVersion()),
		attribute.String("langfuse.environment", p.getEnvironment()),
		attribute.Bool("langfuse.trace.public", false),
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

func (p *LangfuseProvider) StartSpan(ctx context.Context, name string, spanType string, input string, metadata map[string]string) (context.Context, OtelTrace.Span) {
	if p.tracer == nil {
		return ctx, OtelTrace.SpanFromContext(ctx)
	}
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

func (p *LangfuseProvider) StartLLMSpan(ctx context.Context, name string, model string, input string, parameters map[string]interface{}) (context.Context, OtelTrace.Span) {
	if p.tracer == nil {
		return ctx, OtelTrace.SpanFromContext(ctx)
	}
	spanCtx, span := p.tracer.Start(ctx, name)

	// Apply Langfuse LLM generation attributes
	span.SetAttributes(
		attribute.String("langfuse.observation.type", "generation"),
		attribute.String("langfuse.observation.level", "DEFAULT"),
		attribute.String("langfuse.observation.model.name", model),
		attribute.String("langfuse.observation.input", input),
	)
	// Add model parameters in Langfuse format
	if len(parameters) > 0 {
		if paramsJSON, err := json.Marshal(parameters); err == nil {
			span.SetAttributes(
				attribute.String("langfuse.observation.model.parameters", string(paramsJSON)),
			)
		}
	}

	return spanCtx, span
}

func (p *LangfuseProvider) SetOutput(span OtelTrace.Span, output string) {
	span.SetAttributes(
		attribute.String("langfuse.observation.output", output),
		attribute.String("langfuse.trace.output", output), // Will be overridden by child spans
	)
}

func (p *LangfuseProvider) SetTokenUsage(span OtelTrace.Span, promptTokens, completionTokens, reasoningTokens, totalTokens int) {
	// Langfuse usage format
	usageDetails := map[string]int{
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      totalTokens,
		"reasoning_tokens":  reasoningTokens,
	}

	if usageJSON, err := json.Marshal(usageDetails); err == nil {
		span.SetAttributes(
			attribute.String("langfuse.observation.usage_details", string(usageJSON)),
			attribute.Int("llm.token_count.prompt_tokens", promptTokens),
			attribute.Int("llm.token_count.completion_tokens", completionTokens),
			attribute.Int("llm.token_count.total_tokens", totalTokens),
			attribute.Int("llm.token_count.reasoning_tokens", reasoningTokens),
		)
	}
}

func (p *LangfuseProvider) SetDuration(span OtelTrace.Span, duration time.Duration) {
	span.SetAttributes(
		attribute.Float64("duration.seconds", duration.Seconds()),
		attribute.Int64("duration.milliseconds", duration.Milliseconds()),
	)
}

func (p *LangfuseProvider) RecordError(span OtelTrace.Span, err error, level string) {
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

func (p *LangfuseProvider) RecordSuccess(span OtelTrace.Span, message string) {
	span.SetAttributes(
		attribute.String("langfuse.observation.level", "DEFAULT"),
		attribute.String("langfuse.observation.status_message", message),
	)
	span.SetStatus(codes.Ok, message)
}

func (p *LangfuseProvider) Shutdown(ctx context.Context) error {
	if p.cleanup != nil {
		p.cleanup()
		p.cleanup = nil // Prevent double cleanup
	}
	return nil
}

func (p *LangfuseProvider) Close() error {
	return p.Shutdown(context.Background())
}

func (p *LangfuseProvider) GetProvider() TracingProvider {
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
