package observability

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// testConfig returns config with TestMode enabled to avoid Prometheus registration issues
func testConfig() *ObservabilityConfig {
	return &ObservabilityConfig{
		MetricsEnabled:   true,
		TracingEnabled:   true,
		ProfilingEnabled: true,
		LogLevel:         "info",
		ServiceName:      "test-service",
		Environment:      "test",
		SamplingRate:     0.1,
		TestMode:         true,
	}
}

// TestNewFrameworkWithDefaults tests framework initialization with default config
func TestNewFrameworkWithDefaults(t *testing.T) {
	config := testConfig()
	framework, err := NewFramework(config)
	if err != nil {
		t.Fatalf("NewFramework failed with defaults: %v", err)
	}

	if framework == nil {
		t.Error("expected framework to be initialized, got nil")
	}

	if !framework.enabled {
		t.Error("framework should be enabled after creation")
	}

	if framework.config == nil {
		t.Error("framework config should be initialized")
	}

	framework.Close()
}

// TestNewFrameworkWithCustomConfig tests framework with custom configuration
func TestNewFrameworkWithCustomConfig(t *testing.T) {
	config := &ObservabilityConfig{
		MetricsEnabled:   true,
		TracingEnabled:   false,
		ProfilingEnabled: true,
		LogLevel:         "debug",
		ServiceName:      "custom-service",
		Environment:      "test",
		SamplingRate:     0.5,
		TestMode:         true,
	}

	framework, err := NewFramework(config)
	if err != nil {
		t.Fatalf("NewFramework failed: %v", err)
	}

	if !framework.config.MetricsEnabled {
		t.Error("metrics should be enabled")
	}

	if framework.config.TracingEnabled {
		t.Error("tracing should be disabled")
	}

	if !framework.config.ProfilingEnabled {
		t.Error("profiling should be enabled")
	}

	if framework.config.ServiceName != "custom-service" {
		t.Errorf("expected service name 'custom-service', got %s", framework.config.ServiceName)
	}

	framework.Close()
}

// TestStartSpanCreatesValidSpan tests span creation with proper IDs
func TestStartSpanCreatesValidSpan(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	defer framework.Close()

	ctx := context.Background()
	newCtx, span := framework.StartSpan(ctx, "test-operation")

	if span == nil {
		t.Error("expected span to be created, got nil")
	}

	if span.TraceID == "" {
		t.Error("trace ID should not be empty")
	}

	if span.SpanID == "" {
		t.Error("span ID should not be empty")
	}

	if span.Operation != "test-operation" {
		t.Errorf("expected operation 'test-operation', got %s", span.Operation)
	}

	if span.Status != "running" {
		t.Errorf("expected status 'running', got %s", span.Status)
	}

	if span.StartTime.IsZero() {
		t.Error("start time should be set")
	}

	if newCtx == ctx {
		t.Error("context should be modified with span information")
	}

	spanFromCtx, ok := newCtx.Value("span").(string)
	if !ok || spanFromCtx == "" {
		t.Error("context should contain span ID")
	}

	traceFromCtx, ok := newCtx.Value("trace").(string)
	if !ok || traceFromCtx == "" {
		t.Error("context should contain trace ID")
	}
}

// TestStartSpanDisabledTracing tests that no span is created when tracing disabled
func TestStartSpanDisabledTracing(t *testing.T) {
	config := testConfig()
	config.TracingEnabled = false
	framework, _ := NewFramework(config)
	defer framework.Close()

	ctx := context.Background()
	newCtx, span := framework.StartSpan(ctx, "test-operation")

	if span != nil {
		t.Error("expected span to be nil when tracing is disabled")
	}

	if newCtx != ctx {
		t.Error("context should not be modified when tracing is disabled")
	}
}

// TestEndSpanRecordsMetrics tests that span closure records metrics
func TestEndSpanRecordsMetrics(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	defer framework.Close()

	ctx := context.Background()
	newCtx, span := framework.StartSpan(ctx, "test-operation")

	time.Sleep(10 * time.Millisecond)

	framework.EndSpan(newCtx, span, nil)

	framework.tracing.mu.RLock()
	_, exists := framework.tracing.spans[span.SpanID]
	framework.tracing.mu.RUnlock()

	if exists {
		t.Error("span should be removed after EndSpan")
	}

	if span.Status != "success" {
		t.Errorf("expected status 'success', got %s", span.Status)
	}
}

// TestEndSpanWithError tests span closure with error recording
func TestEndSpanWithError(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	defer framework.Close()

	ctx := context.Background()
	newCtx, span := framework.StartSpan(ctx, "test-operation")

	testErr := errors.New("test error")
	framework.EndSpan(newCtx, span, testErr)

	if span.Status != "error" {
		t.Errorf("expected status 'error', got %s", span.Status)
	}
}

// TestEndSpanWithNilSpan tests that EndSpan handles nil span gracefully
func TestEndSpanWithNilSpan(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	defer framework.Close()

	ctx := context.Background()
	framework.EndSpan(ctx, nil, nil)
}

// TestParentChildSpanRelationship tests parent-child span tracking
func TestParentChildSpanRelationship(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	defer framework.Close()

	ctx := context.Background()

	parentCtx, parentSpan := framework.StartSpan(ctx, "parent-operation")
	childCtx, childSpan := framework.StartSpan(parentCtx, "child-operation")

	if childSpan.ParentSpanID == "" {
		t.Error("child span should have parent span ID")
	}

	if childSpan.ParentSpanID != parentSpan.SpanID {
		t.Errorf("child parent ID %s does not match parent span ID %s", childSpan.ParentSpanID, parentSpan.SpanID)
	}

	if childSpan.TraceID != parentSpan.TraceID {
		t.Error("child and parent should have same trace ID")
	}

	framework.EndSpan(childCtx, childSpan, nil)
	framework.EndSpan(parentCtx, parentSpan, nil)
}

// TestRecordMetric tests custom metric recording
func TestRecordMetric(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	defer framework.Close()

	framework.RecordMetric("custom_metric", 42.5, map[string]string{"service": "test"})
}

// TestRecordMetricDisabledMetrics tests metric recording when disabled
func TestRecordMetricDisabledMetrics(t *testing.T) {
	config := testConfig()
	config.MetricsEnabled = false
	framework, _ := NewFramework(config)
	defer framework.Close()

	framework.RecordMetric("custom_metric", 42.5, nil)
}

// TestLogWithDifferentLevels tests logging at different levels
func TestLogWithDifferentLevels(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	defer framework.Close()

	levels := []string{"debug", "info", "warn", "error"}
	for _, level := range levels {
		framework.Log(level, "test message", zap.String("key", "value"))
	}

	framework.Log("unknown", "test message")
}

// TestLogDisabledFramework tests logging when framework is disabled
func TestLogDisabledFramework(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	framework.enabled = false

	framework.Log("info", "test message")
}

// TestProfileRecordsMetrics tests function profiling
func TestProfileRecordsMetrics(t *testing.T) {
	config := testConfig()
	config.ProfilingEnabled = true
	framework, _ := NewFramework(config)
	defer framework.Close()

	callCount := 0
	err := framework.Profile(context.Background(), "test-func", func() error {
		callCount++
		time.Sleep(5 * time.Millisecond)
		return nil
	})

	if err != nil {
		t.Errorf("Profile should not return error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected function to be called once, got %d", callCount)
	}

	framework.profiler.mu.RLock()
	sample, exists := framework.profiler.samples["test-func"]
	framework.profiler.mu.RUnlock()

	if !exists {
		t.Error("profiling sample should be recorded")
	}

	if sample.CallCount != 1 {
		t.Errorf("expected call count 1, got %d", sample.CallCount)
	}

	if sample.Duration == 0 {
		t.Error("duration should be recorded")
	}
}

// TestProfileRecordsError tests profiling with error
func TestProfileRecordsError(t *testing.T) {
	config := testConfig()
	config.ProfilingEnabled = true
	framework, _ := NewFramework(config)
	defer framework.Close()

	testErr := errors.New("test error")
	err := framework.Profile(context.Background(), "test-func", func() error {
		return testErr
	})

	if err != testErr {
		t.Errorf("Profile should return original error: %v", err)
	}

	framework.profiler.mu.RLock()
	sample, exists := framework.profiler.samples["test-func"]
	framework.profiler.mu.RUnlock()

	if !exists {
		t.Error("profiling sample should be recorded even on error")
	}

	if sample.Errors != 1 {
		t.Errorf("expected error count 1, got %d", sample.Errors)
	}
}

// TestProfileDisabled tests profiling when disabled
func TestProfileDisabled(t *testing.T) {
	config := testConfig()
	config.ProfilingEnabled = false
	framework, _ := NewFramework(config)
	defer framework.Close()

	called := false
	err := framework.Profile(context.Background(), "test-func", func() error {
		called = true
		return nil
	})

	if !called {
		t.Error("function should still be called even when profiling disabled")
	}

	if err != nil {
		t.Error("Profile should not return error")
	}
}

// TestGetMetrics returns observability metrics
func TestGetMetrics(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	defer framework.Close()

	metrics := framework.GetMetrics()

	if metrics == nil {
		t.Error("GetMetrics should return metrics map")
	}

	if metricsEnabled, ok := metrics["metrics_enabled"].(bool); !ok || !metricsEnabled {
		t.Error("metrics_enabled should be present and true")
	}

	if tracingEnabled, ok := metrics["tracing_enabled"].(bool); !ok || !tracingEnabled {
		t.Error("tracing_enabled should be present and true")
	}

	if activeSpans, ok := metrics["active_spans"].(int); !ok {
		t.Error("active_spans should be present as int")
	} else if activeSpans != 0 {
		t.Errorf("expected 0 active spans, got %d", activeSpans)
	}
}

// TestGetMetricsWithActiveSpans verifies active span count
func TestGetMetricsWithActiveSpans(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	defer framework.Close()

	ctx := context.Background()
	_, span1 := framework.StartSpan(ctx, "operation1")
	_, span2 := framework.StartSpan(ctx, "operation2")

	metrics := framework.GetMetrics()
	activeSpans := metrics["active_spans"].(int)

	if activeSpans != 2 {
		t.Errorf("expected 2 active spans, got %d", activeSpans)
	}

	framework.EndSpan(ctx, span1, nil)

	metrics = framework.GetMetrics()
	activeSpans = metrics["active_spans"].(int)

	if activeSpans != 1 {
		t.Errorf("expected 1 active span after closing one, got %d", activeSpans)
	}

	framework.EndSpan(ctx, span2, nil)
}

// TestConcurrentSpanOperations tests thread-safe span operations
func TestConcurrentSpanOperations(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	defer framework.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	spanCount := 10

	for i := 0; i < spanCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, span := framework.StartSpan(ctx, fmt.Sprintf("operation-%d", index))
			time.Sleep(time.Duration(index) * time.Millisecond)
			framework.EndSpan(ctx, span, nil)
		}(i)
	}

	wg.Wait()

	metrics := framework.GetMetrics()
	activeSpans := metrics["active_spans"].(int)

	if activeSpans != 0 {
		t.Errorf("expected 0 active spans after concurrent operations, got %d", activeSpans)
	}
}

// TestConcurrentMetricRecording tests thread-safe metric recording
func TestConcurrentMetricRecording(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	defer framework.Close()

	var wg sync.WaitGroup
	metricCount := 10

	for i := 0; i < metricCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			framework.RecordMetric(fmt.Sprintf("metric-%d", index), float64(index), nil)
		}(i)
	}

	wg.Wait()
}

// TestConcurrentProfileOperations tests thread-safe profiling
func TestConcurrentProfileOperations(t *testing.T) {
	config := testConfig()
	config.ProfilingEnabled = true
	framework, _ := NewFramework(config)
	defer framework.Close()

	var wg sync.WaitGroup
	funcCount := 5

	for i := 0; i < funcCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				framework.Profile(context.Background(), fmt.Sprintf("func-%d", index), func() error {
					time.Sleep(time.Millisecond)
					return nil
				})
			}
		}(i)
	}

	wg.Wait()

	framework.profiler.mu.RLock()
	samples := len(framework.profiler.samples)
	framework.profiler.mu.RUnlock()

	if samples != funcCount {
		t.Errorf("expected %d samples, got %d", funcCount, samples)
	}
}

// TestFrameworkClose tests graceful shutdown
func TestFrameworkClose(t *testing.T) {
	framework, _ := NewFramework(testConfig())

	err := framework.Close()
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}

	if framework.enabled {
		t.Error("framework should be disabled after Close")
	}
}

// TestFrameworkCloseDisablesLogging tests that Close disables logging
func TestFrameworkCloseDisablesLogging(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	framework.Close()

	framework.Log("info", "test message after close")
}

// TestSpanContextAttributes tests span context attribute storage
func TestSpanContextAttributes(t *testing.T) {
	framework, _ := NewFramework(testConfig())
	defer framework.Close()

	ctx := context.Background()
	_, span := framework.StartSpan(ctx, "test-operation")

	if span.Attributes == nil {
		t.Error("span attributes should be initialized")
	}

	span.Attributes["user_id"] = "user123"
	span.Attributes["request_id"] = "req456"

	if span.Attributes["user_id"] != "user123" {
		t.Error("span attributes should be retrievable")
	}

	framework.EndSpan(ctx, span, nil)
}

// TestInitializeLoggerInvalidLevel tests logger initialization with invalid level
func TestInitializeLoggerInvalidLevel(t *testing.T) {
	logger, err := initializeLogger("invalid_level", false)
	if err != nil {
		t.Fatalf("initializeLogger should handle invalid level: %v", err)
	}

	if logger == nil {
		t.Error("logger should be initialized even with invalid level")
	}

	_ = logger.Sync()
}

// TestInitializeLoggerProductionConfig tests production logger
func TestInitializeLoggerProductionConfig(t *testing.T) {
	logger, err := initializeLogger("info", true)
	if err != nil {
		t.Fatalf("initializeLogger failed: %v", err)
	}

	if logger == nil {
		t.Error("logger should be initialized")
	}

	_ = logger.Sync()
}

// TestGenerateTraceID tests trace ID generation
func TestGenerateTraceID(t *testing.T) {
	id1 := generateTraceID()
	time.Sleep(1 * time.Nanosecond) // Ensure different nanosecond
	id2 := generateTraceID()

	if id1 == "" {
		t.Error("trace ID should not be empty")
	}

	if id1 == id2 {
		t.Error("trace IDs should be unique")
	}

	if !isNumeric(id1) {
		t.Error("trace ID should be numeric string")
	}
}

// TestGenerateSpanID tests span ID generation
func TestGenerateSpanID(t *testing.T) {
	id1 := generateSpanID()
	time.Sleep(1 * time.Nanosecond) // Ensure different nanosecond
	id2 := generateSpanID()

	if id1 == "" {
		t.Error("span ID should not be empty")
	}

	if id1 == id2 {
		t.Error("span IDs should be unique")
	}

	if !strings.Contains(id1, "-") {
		t.Error("span ID should contain PID separator")
	}
}

// Helper function to check if string is numeric
func isNumeric(s string) bool {
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return len(s) > 0
}
