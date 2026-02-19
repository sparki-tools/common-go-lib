package storm

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

// testLogger creates a test logger
func testLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

// TestNewClient tests client creation
func TestNewClient(t *testing.T) {
	config := &ClientConfig{
		Endpoint: "http://localhost:8080",
		Timeout:  30 * time.Second,
		Logger:   testLogger(),
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if client == nil {
		t.Error("expected client to be initialized")
	}

	if client.endpoint != "http://localhost:8080" {
		t.Errorf("expected endpoint http://localhost:8080, got %s", client.endpoint)
	}

	client.Close()
}

// TestNewClientWithDefaults tests client creation with default config
func TestNewClientWithDefaults(t *testing.T) {
	client, err := NewClient(nil)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if client == nil {
		t.Error("expected client to be initialized")
	}

	if client.endpoint != "http://localhost:8080" {
		t.Errorf("expected default endpoint http://localhost:8080, got %s", client.endpoint)
	}

	client.Close()
}

// TestMetricsPayload tests metrics payload structure
func TestMetricsPayload(t *testing.T) {
	metrics := &MetricsPayload{
		ServiceName: "test-service",
		Timestamp:   time.Now().Unix(),
		Metrics: map[string]interface{}{
			"request_count": 100,
			"error_rate":    0.05,
		},
		Tags: map[string]string{
			"env": "test",
		},
	}

	if metrics.ServiceName != "test-service" {
		t.Error("service name not set correctly")
	}

	if len(metrics.Metrics) != 2 {
		t.Errorf("expected 2 metrics, got %d", len(metrics.Metrics))
	}

	if len(metrics.Tags) != 1 {
		t.Errorf("expected 1 tag, got %d", len(metrics.Tags))
	}
}

// TestTracePayload tests trace payload structure
func TestTracePayload(t *testing.T) {
	trace := &TracePayload{
		TraceID:      "trace-123",
		SpanID:       "span-456",
		ParentSpanID: "parent-789",
		Operation:    "GET /api/users",
		Duration:     150,
		Status:       "success",
		ServiceName:  "test-service",
		Timestamp:    time.Now().Unix(),
		Tags: map[string]interface{}{
			"user_id": "user123",
		},
	}

	if trace.TraceID != "trace-123" {
		t.Error("trace ID not set correctly")
	}

	if trace.ParentSpanID != "parent-789" {
		t.Error("parent span ID not set correctly")
	}

	if trace.Duration != 150 {
		t.Errorf("expected duration 150, got %d", trace.Duration)
	}
}

// TestLogPayload tests log payload structure
func TestLogPayload(t *testing.T) {
	log := &LogPayload{
		Level:       "error",
		Message:     "connection refused",
		ServiceName: "test-service",
		Timestamp:   time.Now().Unix(),
		Fields: map[string]interface{}{
			"error_code": 500,
			"retry":      true,
		},
	}

	if log.Level != "error" {
		t.Error("log level not set correctly")
	}

	if log.Message != "connection refused" {
		t.Error("log message not set correctly")
	}

	if len(log.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(log.Fields))
	}
}

// TestPublishMetricsValidation tests metrics validation
func TestPublishMetricsValidation(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})
	defer client.Close()

	ctx := context.Background()

	// Test nil payload
	err := client.PublishMetrics(ctx, nil)
	if err == nil {
		t.Error("expected error for nil metrics payload")
	}
}

// TestPublishTraceValidation tests trace validation
func TestPublishTraceValidation(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})
	defer client.Close()

	ctx := context.Background()

	// Test nil payload
	err := client.PublishTrace(ctx, nil)
	if err == nil {
		t.Error("expected error for nil trace payload")
	}
}

// TestPublishLogValidation tests log validation
func TestPublishLogValidation(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})
	defer client.Close()

	ctx := context.Background()

	// Test nil payload
	err := client.PublishLog(ctx, nil)
	if err == nil {
		t.Error("expected error for nil log payload")
	}
}

// TestClientTimeout tests client timeout configuration
func TestClientTimeout(t *testing.T) {
	config := &ClientConfig{
		Endpoint: "http://localhost:8080",
		Timeout:  5 * time.Second,
		Logger:   testLogger(),
	}

	client, _ := NewClient(config)
	defer client.Close()

	if client.timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", client.timeout)
	}
}

// TestClientAPIKey tests API key configuration
func TestClientAPIKey(t *testing.T) {
	config := &ClientConfig{
		Endpoint: "http://localhost:8080",
		APIKey:   "test-api-key-123",
		Logger:   testLogger(),
	}

	client, _ := NewClient(config)
	defer client.Close()

	if client.apiKey != "test-api-key-123" {
		t.Errorf("expected API key 'test-api-key-123', got %s", client.apiKey)
	}
}

// TestMetricsPayloadTimestamp tests automatic timestamp assignment
func TestMetricsPayloadTimestamp(t *testing.T) {
	metrics := &MetricsPayload{
		ServiceName: "test-service",
		Metrics:     make(map[string]interface{}),
	}

	if metrics.Timestamp != 0 {
		t.Error("expected timestamp to be zero initially")
	}

	metrics.Timestamp = time.Now().Unix()
	if metrics.Timestamp == 0 {
		t.Error("timestamp should be set")
	}
}

// TestTracePayloadWithoutParent tests trace without parent span
func TestTracePayloadWithoutParent(t *testing.T) {
	trace := &TracePayload{
		TraceID:     "trace-123",
		SpanID:      "span-456",
		Operation:   "GET /api/users",
		Duration:    100,
		Status:      "success",
		ServiceName: "test-service",
	}

	if trace.ParentSpanID != "" {
		t.Error("parent span ID should be empty for root span")
	}
}

// TestLogPayloadLevels tests different log levels
func TestLogPayloadLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error", "fatal"}

	for _, level := range levels {
		log := &LogPayload{
			Level:       level,
			Message:     "test message",
			ServiceName: "test-service",
		}

		if log.Level != level {
			t.Errorf("expected level %s, got %s", level, log.Level)
		}
	}
}

// TestClientClose tests graceful client closure
func TestClientClose(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})

	err := client.Close()
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}
}

// TestMultiplePayloads tests creating multiple payloads
func TestMultiplePayloads(t *testing.T) {
	metrics := &MetricsPayload{ServiceName: "svc1"}
	trace := &TracePayload{TraceID: "trace1"}
	log := &LogPayload{Level: "info"}

	if metrics.ServiceName != "svc1" {
		t.Error("metrics service name incorrect")
	}

	if trace.TraceID != "trace1" {
		t.Error("trace ID incorrect")
	}

	if log.Level != "info" {
		t.Error("log level incorrect")
	}
}
