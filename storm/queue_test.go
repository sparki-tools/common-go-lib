package storm

import (
	"context"
	"testing"
	"time"
)

// TestNewQueue tests queue creation
func TestNewQueue(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})
	defer client.Close()

	config := &QueueConfig{
		BatchSize:     50,
		FlushInterval: 2 * time.Second,
		Workers:       2,
	}

	queue, err := NewQueue(client, config)
	if err != nil {
		t.Fatalf("NewQueue failed: %v", err)
	}

	if queue == nil {
		t.Error("expected queue to be initialized")
	}

	defer queue.Close(5 * time.Second)
}

// TestNewQueueWithDefaults tests queue with default config
func TestNewQueueWithDefaults(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})
	defer client.Close()

	queue, err := NewQueue(client, nil)
	if err != nil {
		t.Fatalf("NewQueue failed: %v", err)
	}

	if queue == nil {
		t.Error("expected queue to be initialized")
	}

	if queue.config.BatchSize != 100 {
		t.Errorf("expected batch size 100, got %d", queue.config.BatchSize)
	}

	defer queue.Close(5 * time.Second)
}

// TestQueueEnqueueMetrics tests enqueueing metrics
func TestQueueEnqueueMetrics(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})
	defer client.Close()

	queue, _ := NewQueue(client, nil)
	defer queue.Close(5 * time.Second)

	ctx := context.Background()
	metrics := &MetricsPayload{
		ServiceName: "test-service",
		Metrics:     make(map[string]interface{}),
	}

	err := queue.EnqueueMetrics(ctx, metrics)
	if err != nil {
		t.Errorf("EnqueueMetrics failed: %v", err)
	}
}

// TestQueueEnqueueTrace tests enqueueing trace
func TestQueueEnqueueTrace(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})
	defer client.Close()

	queue, _ := NewQueue(client, nil)
	defer queue.Close(5 * time.Second)

	ctx := context.Background()
	trace := &TracePayload{
		TraceID:     "trace-123",
		SpanID:      "span-456",
		Operation:   "test",
		ServiceName: "test-service",
	}

	err := queue.EnqueueTrace(ctx, trace)
	if err != nil {
		t.Errorf("EnqueueTrace failed: %v", err)
	}
}

// TestQueueEnqueueLog tests enqueueing log
func TestQueueEnqueueLog(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})
	defer client.Close()

	queue, _ := NewQueue(client, nil)
	defer queue.Close(5 * time.Second)

	ctx := context.Background()
	log := &LogPayload{
		Level:       "info",
		Message:     "test message",
		ServiceName: "test-service",
	}

	err := queue.EnqueueLog(ctx, log)
	if err != nil {
		t.Errorf("EnqueueLog failed: %v", err)
	}
}

// TestQueueEnqueueValidation tests queue validation
func TestQueueEnqueueValidation(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})
	defer client.Close()

	queue, _ := NewQueue(client, nil)
	defer queue.Close(5 * time.Second)

	ctx := context.Background()

	// Test nil metrics
	err := queue.EnqueueMetrics(ctx, nil)
	if err == nil {
		t.Error("expected error for nil metrics")
	}

	// Test nil trace
	err = queue.EnqueueTrace(ctx, nil)
	if err == nil {
		t.Error("expected error for nil trace")
	}

	// Test nil log
	err = queue.EnqueueLog(ctx, nil)
	if err == nil {
		t.Error("expected error for nil log")
	}
}

// TestQueueStats tests queue statistics
func TestQueueStats(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})
	defer client.Close()

	queue, _ := NewQueue(client, nil)
	defer queue.Close(5 * time.Second)

	stats := queue.Stats()

	if stats == nil {
		t.Error("expected stats map")
	}

	if isRunning, ok := stats["is_running"].(bool); !ok || !isRunning {
		t.Error("queue should be running")
	}

	if queueLen, ok := stats["queue_length"].(int); !ok || queueLen < 0 {
		t.Error("queue length should be non-negative")
	}
}

// TestQueueClose tests graceful queue shutdown
func TestQueueClose(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})
	defer client.Close()

	queue, _ := NewQueue(client, nil)

	err := queue.Close(5 * time.Second)
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}

	if queue.isRunning {
		t.Error("queue should not be running after close")
	}
}

// TestQueueNotRunningError tests error when queue is not running
func TestQueueNotRunningError(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})
	defer client.Close()

	queue, _ := NewQueue(client, nil)
	queue.Close(5 * time.Second)

	ctx := context.Background()
	metrics := &MetricsPayload{ServiceName: "test"}

	err := queue.EnqueueMetrics(ctx, metrics)
	if err == nil {
		t.Error("expected error when queue is not running")
	}
}

// TestQueueContextCancellation tests context cancellation handling
func TestQueueContextCancellation(t *testing.T) {
	client, _ := NewClient(&ClientConfig{Logger: testLogger()})
	defer client.Close()

	queue, _ := NewQueue(client, nil)
	defer queue.Close(5 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	metrics := &MetricsPayload{ServiceName: "test"}

	err := queue.EnqueueMetrics(ctx, metrics)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

// TestQueueConfig tests queue configuration
func TestQueueConfig(t *testing.T) {
	config := &QueueConfig{
		BatchSize:     50,
		FlushInterval: 3 * time.Second,
		Workers:       4,
		QueueCapacity: 5000,
	}

	if config.BatchSize != 50 {
		t.Errorf("expected batch size 50, got %d", config.BatchSize)
	}

	if config.Workers != 4 {
		t.Errorf("expected 4 workers, got %d", config.Workers)
	}
}

// TestDefaultQueueConfig tests default queue config
func TestDefaultQueueConfig(t *testing.T) {
	config := DefaultQueueConfig()

	if config.BatchSize != 100 {
		t.Errorf("expected default batch size 100, got %d", config.BatchSize)
	}

	if config.Workers != 4 {
		t.Errorf("expected default workers 4, got %d", config.Workers)
	}

	if config.FlushInterval != 5*time.Second {
		t.Errorf("expected default flush interval 5s, got %v", config.FlushInterval)
	}
}
