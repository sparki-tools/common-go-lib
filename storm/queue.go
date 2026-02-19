package storm

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// QueueConfig configures the Storm queue
type QueueConfig struct {
	// Maximum items to batch before sending
	BatchSize int

	// Maximum time to wait before sending batch
	FlushInterval time.Duration

	// Number of worker goroutines
	Workers int

	// Queue capacity
	QueueCapacity int
}

// DefaultQueueConfig returns sensible defaults
func DefaultQueueConfig() *QueueConfig {
	return &QueueConfig{
		BatchSize:     100,
		FlushInterval: 5 * time.Second,
		Workers:       4,
		QueueCapacity: 10000,
	}
}

// QueueItem represents an item in the queue
type QueueItem struct {
	ItemType string      // "metric", "trace", "log"
	Payload  interface{} // MetricsPayload, TracePayload, or LogPayload
}

// Queue provides batching and async publishing to Storm
type Queue struct {
	client       *Client
	config       *QueueConfig
	queue        chan *QueueItem
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.RWMutex
	isRunning    bool
	batches      map[string][]*QueueItem // Batch by type
	lastFlush    time.Time
	flushTicker  *time.Ticker
	errorCount   int64
	processCount int64
}

// NewQueue creates a new Storm queue
func NewQueue(client *Client, config *QueueConfig) (*Queue, error) {
	if client == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}

	if config == nil {
		config = DefaultQueueConfig()
	}

	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}

	if config.FlushInterval == 0 {
		config.FlushInterval = 5 * time.Second
	}

	if config.Workers <= 0 {
		config.Workers = 4
	}

	if config.QueueCapacity <= 0 {
		config.QueueCapacity = 10000
	}

	ctx, cancel := context.WithCancel(context.Background())

	q := &Queue{
		client:     client,
		config:     config,
		queue:      make(chan *QueueItem, config.QueueCapacity),
		ctx:        ctx,
		cancel:     cancel,
		batches:    make(map[string][]*QueueItem),
		lastFlush:  time.Now(),
		flushTicker: time.NewTicker(config.FlushInterval),
	}

	// Start workers
	for i := 0; i < config.Workers; i++ {
		q.wg.Add(1)
		go q.worker()
	}

	// Start flush ticker
	q.wg.Add(1)
	go q.flushWorker()

	q.isRunning = true
	return q, nil
}

// worker processes items from the queue
func (q *Queue) worker() {
	defer q.wg.Done()

	for {
		select {
		case item := <-q.queue:
			if item == nil {
				return
			}

			q.mu.Lock()
			q.batches[item.ItemType] = append(q.batches[item.ItemType], item)
			shouldFlush := len(q.batches[item.ItemType]) >= q.config.BatchSize
			q.mu.Unlock()

			if shouldFlush {
				q.flush(item.ItemType)
			}

		case <-q.ctx.Done():
			return
		}
	}
}

// flushWorker periodically flushes batches
func (q *Queue) flushWorker() {
	defer q.wg.Done()

	for {
		select {
		case <-q.flushTicker.C:
			q.flushAll()

		case <-q.ctx.Done():
			return
		}
	}
}

// flush sends a batch of items
func (q *Queue) flush(itemType string) {
	q.mu.Lock()
	items := q.batches[itemType]
	q.batches[itemType] = nil
	q.mu.Unlock()

	if len(items) == 0 {
		return
	}

	// Send items in parallel based on type
	for _, item := range items {
		ctx, cancel := context.WithTimeout(q.ctx, 30*time.Second)

		var err error
		switch item.ItemType {
		case "metric":
			if payload, ok := item.Payload.(*MetricsPayload); ok {
				err = q.client.PublishMetrics(ctx, payload)
			}
		case "trace":
			if payload, ok := item.Payload.(*TracePayload); ok {
				err = q.client.PublishTrace(ctx, payload)
			}
		case "log":
			if payload, ok := item.Payload.(*LogPayload); ok {
				err = q.client.PublishLog(ctx, payload)
			}
		}

		cancel()

		if err != nil {
			q.mu.Lock()
			q.errorCount++
			q.mu.Unlock()
		} else {
			q.mu.Lock()
			q.processCount++
			q.mu.Unlock()
		}
	}

	q.mu.Lock()
	q.lastFlush = time.Now()
	q.mu.Unlock()
}

// flushAll flushes all batches
func (q *Queue) flushAll() {
	q.mu.RLock()
	itemTypes := make([]string, 0, len(q.batches))
	for itemType := range q.batches {
		itemTypes = append(itemTypes, itemType)
	}
	q.mu.RUnlock()

	for _, itemType := range itemTypes {
		q.flush(itemType)
	}
}

// EnqueueMetrics adds metrics to queue
func (q *Queue) EnqueueMetrics(ctx context.Context, metrics *MetricsPayload) error {
	if !q.isRunning {
		return fmt.Errorf("queue is not running")
	}

	if metrics == nil {
		return fmt.Errorf("metrics payload cannot be nil")
	}

	item := &QueueItem{
		ItemType: "metric",
		Payload:  metrics,
	}

	select {
	case q.queue <- item:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("queue is full")
	}
}

// EnqueueTrace adds trace to queue
func (q *Queue) EnqueueTrace(ctx context.Context, trace *TracePayload) error {
	if !q.isRunning {
		return fmt.Errorf("queue is not running")
	}

	if trace == nil {
		return fmt.Errorf("trace payload cannot be nil")
	}

	item := &QueueItem{
		ItemType: "trace",
		Payload:  trace,
	}

	select {
	case q.queue <- item:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("queue is full")
	}
}

// EnqueueLog adds log to queue
func (q *Queue) EnqueueLog(ctx context.Context, log *LogPayload) error {
	if !q.isRunning {
		return fmt.Errorf("queue is not running")
	}

	if log == nil {
		return fmt.Errorf("log payload cannot be nil")
	}

	item := &QueueItem{
		ItemType: "log",
		Payload:  log,
	}

	select {
	case q.queue <- item:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("queue is full")
	}
}

// Stats returns queue statistics
func (q *Queue) Stats() map[string]interface{} {
	q.mu.RLock()
	defer q.mu.RUnlock()

	queueLen := len(q.queue)
	return map[string]interface{}{
		"is_running":     q.isRunning,
		"queue_length":   queueLen,
		"queue_capacity": q.config.QueueCapacity,
		"processed":      q.processCount,
		"errors":         q.errorCount,
		"last_flush":     q.lastFlush,
	}
}

// Close gracefully shuts down the queue
func (q *Queue) Close(timeout time.Duration) error {
	q.mu.Lock()
	if !q.isRunning {
		q.mu.Unlock()
		return fmt.Errorf("queue is not running")
	}
	q.isRunning = false
	q.mu.Unlock()

	// Flush remaining items
	q.flushAll()

	// Stop ticker
	q.flushTicker.Stop()

	// Cancel context
	q.cancel()

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("queue shutdown timeout exceeded")
	}
}
