// Package observability provides unified observability (metrics, tracing, logging)
// tracing.go implements OpenTelemetry SDK integration for distributed tracing
package observability

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TracerConfig holds configuration for OpenTelemetry tracing
type TracerConfig struct {
	// ServiceName is the name of the service (required)
	ServiceName string

	// ServiceVersion is the version of the service
	ServiceVersion string

	// Environment is the deployment environment (e.g., production, staging)
	Environment string

	// OTELEndpoint is the OTLP collector endpoint (default: otel-collector.observability.svc.cluster.local:4317)
	OTELEndpoint string

	// SamplingRate is the probability of sampling a trace (0.0-1.0)
	// Note: Tail-based sampling is done at collector level, this is head-based fallback
	SamplingRate float64

	// Enabled enables/disables tracing
	Enabled bool

	// Insecure disables TLS for gRPC connection (true for in-cluster communication)
	Insecure bool

	// Logger for tracing initialization logs
	Logger *zap.Logger
}

// TracerProvider wraps the OTEL TracerProvider with Sparki-specific functionality
type TracerProvider struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	config   *TracerConfig
	logger   *zap.Logger
}

// DefaultTracerConfig returns a TracerConfig with sensible defaults
func DefaultTracerConfig(serviceName string) *TracerConfig {
	return &TracerConfig{
		ServiceName:    serviceName,
		ServiceVersion: getEnvOrDefault("SERVICE_VERSION", "0.0.0"),
		Environment:    getEnvOrDefault("ENVIRONMENT", "development"),
		OTELEndpoint:   getEnvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector.observability.svc.cluster.local:4317"),
		SamplingRate:   1.0, // Sample all at head, collector does tail-based sampling
		Enabled:        getEnvOrDefault("OTEL_TRACING_ENABLED", "true") == "true",
		Insecure:       true, // In-cluster communication, Istio handles mTLS
	}
}

// InitTracer initializes OpenTelemetry tracing with the given configuration
// Returns a TracerProvider that should be shut down on application exit
func InitTracer(ctx context.Context, config *TracerConfig) (*TracerProvider, error) {
	if config == nil {
		return nil, fmt.Errorf("tracer config cannot be nil")
	}

	if !config.Enabled {
		if config.Logger != nil {
			config.Logger.Info("tracing disabled, using noop tracer")
		}
		return &TracerProvider{
			tracer: trace.NewNoopTracerProvider().Tracer(config.ServiceName),
			config: config,
			logger: config.Logger,
		}, nil
	}

	// Create OTLP exporter
	var opts []otlptracegrpc.Option
	opts = append(opts, otlptracegrpc.WithEndpoint(config.OTELEndpoint))

	if config.Insecure {
		opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	// Add retry configuration
	opts = append(opts, otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
		Enabled:         true,
		InitialInterval: 500 * time.Millisecond,
		MaxInterval:     5 * time.Second,
		MaxElapsedTime:  30 * time.Second,
	}))

	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(opts...))
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironment(config.Environment),
			attribute.String("service.namespace", "sparki"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create sampler based on config
	var sampler sdktrace.Sampler
	if config.SamplingRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if config.SamplingRate <= 0.0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(config.SamplingRate)
	}

	// Create trace provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
			sdktrace.WithMaxQueueSize(2048),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sampler)),
	)

	// Set global tracer provider and propagator
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create tracer
	tracer := provider.Tracer(
		config.ServiceName,
		trace.WithInstrumentationVersion(config.ServiceVersion),
	)

	tp := &TracerProvider{
		provider: provider,
		tracer:   tracer,
		config:   config,
		logger:   config.Logger,
	}

	if config.Logger != nil {
		config.Logger.Info("tracing initialized",
			zap.String("service", config.ServiceName),
			zap.String("endpoint", config.OTELEndpoint),
			zap.Float64("sampling_rate", config.SamplingRate),
		)
	}

	return tp, nil
}

// Tracer returns the underlying OTEL tracer
func (tp *TracerProvider) Tracer() trace.Tracer {
	return tp.tracer
}

// Shutdown gracefully shuts down the tracer provider
func (tp *TracerProvider) Shutdown(ctx context.Context) error {
	if tp.provider == nil {
		return nil
	}

	if tp.logger != nil {
		tp.logger.Info("shutting down tracer provider")
	}

	return tp.provider.Shutdown(ctx)
}

// StartSpan starts a new span with the given name and options
func (tp *TracerProvider) StartSpan(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return tp.tracer.Start(ctx, spanName, opts...)
}

// SpanFromContext returns the current span from context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// TraceIDFromContext extracts the trace ID from context
func TraceIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().HasTraceID() {
		return ""
	}
	return span.SpanContext().TraceID().String()
}

// SpanIDFromContext extracts the span ID from context
func SpanIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().HasSpanID() {
		return ""
	}
	return span.SpanContext().SpanID().String()
}

// AddSpanEvent adds an event to the current span
func AddSpanEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetSpanError marks the span as errored and records the error
func SetSpanError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// SetSpanAttributes sets attributes on the current span
func SetSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrs...)
}

// WithSpan executes a function within a new span
func (tp *TracerProvider) WithSpan(ctx context.Context, spanName string, fn func(context.Context) error, opts ...trace.SpanStartOption) error {
	ctx, span := tp.tracer.Start(ctx, spanName, opts...)
	defer span.End()

	if err := fn(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "")
	return nil
}

// Common span attribute helpers

// HTTPServerAttributes returns common attributes for HTTP server spans
func HTTPServerAttributes(method, path string, statusCode int) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.HTTPMethod(method),
		semconv.HTTPRoute(path),
		semconv.HTTPStatusCode(statusCode),
	}
}

// HTTPClientAttributes returns common attributes for HTTP client spans
func HTTPClientAttributes(method, url string, statusCode int) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.HTTPMethod(method),
		semconv.HTTPURL(url),
		semconv.HTTPStatusCode(statusCode),
	}
}

// DatabaseAttributes returns common attributes for database spans
func DatabaseAttributes(system, operation, table string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.DBSystemKey.String(system),
		semconv.DBOperationKey.String(operation),
		semconv.DBSQLTableKey.String(table),
	}
}

// MessagingAttributes returns common attributes for messaging spans
func MessagingAttributes(system, destination, operation string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.MessagingSystemKey.String(system),
		semconv.MessagingDestinationName(destination),
		semconv.MessagingOperationKey.String(operation),
	}
}

// SparkiAttributes returns Sparki-specific span attributes
func SparkiAttributes(projectID, buildID, userID string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{}
	if projectID != "" {
		attrs = append(attrs, attribute.String("sparki.project_id", projectID))
	}
	if buildID != "" {
		attrs = append(attrs, attribute.String("sparki.build_id", buildID))
	}
	if userID != "" {
		attrs = append(attrs, attribute.String("sparki.user_id", userID))
	}
	return attrs
}

// Helper functions

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
