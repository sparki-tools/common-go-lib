package observability

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// Framework provides unified observability (metrics, tracing, logging, profiling)
// as a core framework feature, not bolted on
type Framework struct {
	// Core components
	logger   *zap.Logger
	metrics  *MetricsCollector
	tracing  *TracingCollector
	profiler *ProfilerCollector

	// Configuration
	config *ObservabilityConfig

	// State
	mu      sync.RWMutex
	enabled bool
}

// ObservabilityConfig defines observability behavior
type ObservabilityConfig struct {
	// Metrics
	MetricsEnabled bool
	MetricsPort   int
	MetricsPath   string

	// Tracing
	TracingEnabled  bool
	TracingEndpoint string
	SamplingRate    float64

	// Logging
	LogLevel string
	LogJSON  bool

	// Profiling
	ProfilingEnabled bool
	ProfilingPort   int

	// General
	ServiceName string
	Environment string
	Version     string

	// Testing
	TestMode bool
}

// MetricsCollector handles prometheus metrics
type MetricsCollector struct {
	// Request metrics
	requestDuration prometheus.Histogram
	requestCount    prometheus.Counter
	requestErrors   prometheus.Counter

	// System metrics
	goroutines prometheus.Gauge
	memory     prometheus.Gauge
	cpuUsage   prometheus.Gauge

	// Custom metrics
	customMetrics map[string]prometheus.Collector
	mu            sync.RWMutex
}

// TracingCollector handles distributed tracing
type TracingCollector struct {
	spans         map[string]*SpanContext
	mu            sync.RWMutex
	samplingRate  float64
	enabled       bool
}

// SpanContext represents active span
type SpanContext struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Operation    string
	StartTime    time.Time
	Attributes   map[string]interface{}
	Status       string
}

// ProfilerCollector handles performance profiling
type ProfilerCollector struct {
	enabled      bool
	samples      map[string]*ProfileSample
	mu           sync.RWMutex
	sampleRate   float64
}

// ProfileSample represents profiling data point
type ProfileSample struct {
	Function    string
	Duration    time.Duration
	CallCount   int
	Errors      int
	Memory      int64
}

// NewFramework creates a new observability framework
func NewFramework(config *ObservabilityConfig) (*Framework, error) {
	if config == nil {
		config = &ObservabilityConfig{
			MetricsEnabled:   true,
			TracingEnabled:   true,
			ProfilingEnabled: false,
			LogLevel:         "info",
			ServiceName:      "sparki-service",
			SamplingRate:     0.1,
		}
	}

	// Initialize logger
	logger, err := initializeLogger(config.LogLevel, config.LogJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Initialize metrics
	var requestDuration prometheus.Histogram
	var requestCount prometheus.Counter
	var requestErrors prometheus.Counter
	var goroutines prometheus.Gauge
	var memory prometheus.Gauge
	var cpuUsage prometheus.Gauge

	if !config.TestMode {
		requestDuration = promauto.NewHistogram(prometheus.HistogramOpts{
			Name: fmt.Sprintf("%s_request_duration_seconds", config.ServiceName),
			Help: "Request duration in seconds",
		})
		requestCount = promauto.NewCounter(prometheus.CounterOpts{
			Name: fmt.Sprintf("%s_requests_total", config.ServiceName),
			Help: "Total number of requests",
		})
		requestErrors = promauto.NewCounter(prometheus.CounterOpts{
			Name: fmt.Sprintf("%s_request_errors_total", config.ServiceName),
			Help: "Total number of request errors",
		})
		goroutines = promauto.NewGauge(prometheus.GaugeOpts{
			Name: fmt.Sprintf("%s_goroutines", config.ServiceName),
			Help: "Number of active goroutines",
		})
		memory = promauto.NewGauge(prometheus.GaugeOpts{
			Name: fmt.Sprintf("%s_memory_bytes", config.ServiceName),
			Help: "Memory usage in bytes",
		})
		cpuUsage = promauto.NewGauge(prometheus.GaugeOpts{
			Name: fmt.Sprintf("%s_cpu_usage_percent", config.ServiceName),
			Help: "CPU usage percentage",
		})
	}

	metrics := &MetricsCollector{
		requestDuration: requestDuration,
		requestCount:    requestCount,
		requestErrors:   requestErrors,
		goroutines:      goroutines,
		memory:          memory,
		cpuUsage:        cpuUsage,
		customMetrics:   make(map[string]prometheus.Collector),
	}

	// Initialize tracing
	tracing := &TracingCollector{
		spans:        make(map[string]*SpanContext),
		samplingRate: config.SamplingRate,
		enabled:      config.TracingEnabled,
	}

	// Initialize profiler
	profiler := &ProfilerCollector{
		enabled:    config.ProfilingEnabled,
		samples:    make(map[string]*ProfileSample),
		sampleRate: 0.1, // Sample 10% of calls by default
	}

	framework := &Framework{
		logger:    logger,
		metrics:   metrics,
		tracing:   tracing,
		profiler:  profiler,
		config:    config,
		enabled:   true,
	}

	logger.Info("observability framework initialized",
		zap.String("service", config.ServiceName),
		zap.String("environment", config.Environment),
		zap.Bool("metrics_enabled", config.MetricsEnabled),
		zap.Bool("tracing_enabled", config.TracingEnabled),
	)

	return framework, nil
}

// StartSpan creates a new tracing span
func (f *Framework) StartSpan(ctx context.Context, operation string) (context.Context, *SpanContext) {
	if !f.config.TracingEnabled {
		return ctx, nil
	}

	f.tracing.mu.Lock()
	defer f.tracing.mu.Unlock()

	// Generate span ID
	spanID := generateSpanID()

	// Get parent span and trace from context if exists
	parentSpanID := ""
	traceID := generateTraceID()
	if parent, ok := ctx.Value("span").(string); ok {
		parentSpanID = parent
	}
	if trace, ok := ctx.Value("trace").(string); ok {
		traceID = trace
	}

	span := &SpanContext{
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: parentSpanID,
		Operation:    operation,
		StartTime:    time.Now(),
		Attributes:   make(map[string]interface{}),
		Status:       "running",
	}

	// Store span
	f.tracing.spans[spanID] = span

	// Add to context
	newCtx := context.WithValue(ctx, "span", spanID)
	newCtx = context.WithValue(newCtx, "trace", traceID)

	// Log span start
	f.logger.Debug("span started",
		zap.String("trace_id", traceID),
		zap.String("span_id", spanID),
		zap.String("operation", operation),
	)

	return newCtx, span
}

// EndSpan closes a tracing span
func (f *Framework) EndSpan(ctx context.Context, span *SpanContext, err error) {
	if !f.config.TracingEnabled || span == nil {
		return
	}

	f.tracing.mu.Lock()
	defer f.tracing.mu.Unlock()

	duration := time.Since(span.StartTime)

	if err != nil {
		span.Status = "error"
		if f.metrics.requestErrors != nil {
			f.metrics.requestErrors.Inc()
		}
		f.logger.Error("span ended with error",
			zap.String("trace_id", span.TraceID),
			zap.String("span_id", span.SpanID),
			zap.Error(err),
			zap.Duration("duration", duration),
		)
	} else {
		span.Status = "success"
		f.logger.Debug("span ended",
			zap.String("trace_id", span.TraceID),
			zap.String("span_id", span.SpanID),
			zap.Duration("duration", duration),
		)
	}

	// Record metrics
	if f.metrics.requestDuration != nil {
		f.metrics.requestDuration.Observe(duration.Seconds())
	}
	if f.metrics.requestCount != nil {
		f.metrics.requestCount.Inc()
	}

	// Remove span
	delete(f.tracing.spans, span.SpanID)
}

// RecordMetric records a custom metric
func (f *Framework) RecordMetric(name string, value float64, labels map[string]string) {
	if !f.config.MetricsEnabled {
		return
	}

	f.metrics.mu.Lock()
	defer f.metrics.mu.Unlock()

	// Store metric value (simplified - in production use prometheus directly)
	f.logger.Debug("metric recorded",
		zap.String("name", name),
		zap.Float64("value", value),
	)
}

// Log logs a message with context
func (f *Framework) Log(level string, msg string, fields ...zap.Field) {
	if !f.enabled {
		return
	}

	switch level {
	case "debug":
		f.logger.Debug(msg, fields...)
	case "info":
		f.logger.Info(msg, fields...)
	case "warn":
		f.logger.Warn(msg, fields...)
	case "error":
		f.logger.Error(msg, fields...)
	default:
		f.logger.Info(msg, fields...)
	}
}

// Profile records profiling data for a function
func (f *Framework) Profile(ctx context.Context, funcName string, fn func() error) error {
	if !f.config.ProfilingEnabled {
		return fn()
	}

	start := time.Now()
	err := fn()
	duration := time.Since(start)

	f.profiler.mu.Lock()
	defer f.profiler.mu.Unlock()

	sample, exists := f.profiler.samples[funcName]
	if !exists {
		sample = &ProfileSample{
			Function: funcName,
		}
		f.profiler.samples[funcName] = sample
	}

	sample.CallCount++
	sample.Duration += duration

	if err != nil {
		sample.Errors++
	}

	f.logger.Debug("function profiled",
		zap.String("function", funcName),
		zap.Duration("duration", duration),
		zap.Error(err),
	)

	return err
}

// GetMetrics returns current metrics
func (f *Framework) GetMetrics() map[string]interface{} {
	f.metrics.mu.RLock()
	defer f.metrics.mu.RUnlock()

	return map[string]interface{}{
		"metrics_enabled": f.config.MetricsEnabled,
		"tracing_enabled": f.config.TracingEnabled,
		"profiling_enabled": f.config.ProfilingEnabled,
		"active_spans": len(f.tracing.spans),
	}
}

// Close gracefully shuts down observability
func (f *Framework) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.enabled = false

	// Flush logger
	_ = f.logger.Sync()

	f.logger.Info("observability framework shut down")

	return nil
}

// Helper functions

func initializeLogger(level string, json bool) (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	if !json {
		config = zap.NewDevelopmentConfig()
	}

	// Set level
	switch level {
	case "debug":
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case "warn":
		config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "error":
		config.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	}

	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return logger, nil
}

func generateTraceID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func generateSpanID() string {
	return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
}
