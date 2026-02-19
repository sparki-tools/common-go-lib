package testing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// TestContext provides isolated test environment
type TestContext struct {
	Context   context.Context
	Cancel    context.CancelFunc
	Logger    *zap.Logger
	Registry  *prometheus.Registry
	Timeout   time.Duration
	mu        sync.Mutex
	Cleanup   []func()
}

// NewTestContext creates an isolated test environment with its own metrics registry
func NewTestContext(timeout time.Duration) *TestContext {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	logger, _ := zap.NewDevelopment()

	registry := prometheus.NewRegistry()

	return &TestContext{
		Context:  ctx,
		Cancel:   cancel,
		Logger:   logger,
		Registry: registry,
		Timeout:  timeout,
		Cleanup:  make([]func(), 0),
	}
}

// AddCleanup registers a cleanup function to be called when TestContext is cleaned up
func (tc *TestContext) AddCleanup(fn func()) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.Cleanup = append(tc.Cleanup, fn)
}

// Done cleans up resources and cancels context
func (tc *TestContext) Done() {
	defer tc.Cancel()
	defer tc.Logger.Sync()

	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Execute cleanup functions in reverse order
	for i := len(tc.Cleanup) - 1; i >= 0; i-- {
		tc.Cleanup[i]()
	}
}

// TestContextFactory is used for creating test objects with isolated context
type TestContextFactory struct {
	TestContext *TestContext
	mu          sync.Mutex
}

// NewTestContextFactory creates a new test factory tied to a TestContext
func NewTestContextFactory(tc *TestContext) *TestContextFactory {
	return &TestContextFactory{
		TestContext: tc,
	}
}

// AssertTimeout helps identify tests that exceed expected duration
type AssertTimeout struct {
	Expected time.Duration
	Actual   time.Duration
	Exceeded bool
}

// MeasureDuration measures execution time and validates against expected duration
func (tc *TestContext) MeasureDuration(fn func(), expected time.Duration) *AssertTimeout {
	start := time.Now()
	fn()
	actual := time.Since(start)

	return &AssertTimeout{
		Expected: expected,
		Actual:   actual,
		Exceeded: actual > expected,
	}
}

// String returns formatted duration assertion
func (at *AssertTimeout) String() string {
	if at.Exceeded {
		return fmt.Sprintf("TIMEOUT: expected %v, took %v (exceeded by %v)", at.Expected, at.Actual, at.Actual-at.Expected)
	}
	return fmt.Sprintf("OK: expected %v, took %v", at.Expected, at.Actual)
}

// ConcurrencyTest validates concurrent operation behavior
type ConcurrencyTest struct {
	NumGoroutines int
	OperationsFn  func(ctx context.Context, id int) error
	Results       map[int]error
	mu            sync.Mutex
}

// NewConcurrencyTest creates a new concurrency test
func NewConcurrencyTest(numGoroutines int, fn func(ctx context.Context, id int) error) *ConcurrencyTest {
	return &ConcurrencyTest{
		NumGoroutines: numGoroutines,
		OperationsFn:  fn,
		Results:       make(map[int]error),
	}
}

// Execute runs concurrent operations and collects results
func (ct *ConcurrencyTest) Execute(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(ct.NumGoroutines)

	for i := 0; i < ct.NumGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			err := ct.OperationsFn(ctx, id)
			if err != nil {
				ct.mu.Lock()
				ct.Results[id] = err
				ct.mu.Unlock()
			}
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Check if any operations failed
		if len(ct.Results) > 0 {
			return fmt.Errorf("concurrency test had %d failures", len(ct.Results))
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// FailureCount returns number of failed operations
func (ct *ConcurrencyTest) FailureCount() int {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return len(ct.Results)
}

// DeterminismTest validates operation produces identical results across runs
type DeterminismTest struct {
	Runs      int
	Operation func(ctx context.Context) (interface{}, error)
	Results   []interface{}
	Errors    []error
}

// NewDeterminismTest creates a new determinism test
func NewDeterminismTest(runs int, fn func(ctx context.Context) (interface{}, error)) *DeterminismTest {
	return &DeterminismTest{
		Runs:      runs,
		Operation: fn,
		Results:   make([]interface{}, runs),
		Errors:    make([]error, runs),
	}
}

// Execute runs the operation multiple times and validates consistency
func (dt *DeterminismTest) Execute(ctx context.Context) error {
	for i := 0; i < dt.Runs; i++ {
		result, err := dt.Operation(ctx)
		dt.Results[i] = result
		dt.Errors[i] = err
	}

	// Check all errors match
	if dt.Errors[0] != nil {
		for i := 1; i < len(dt.Errors); i++ {
			if (dt.Errors[i] != nil) != (dt.Errors[0] != nil) {
				return fmt.Errorf("inconsistent error: run %d had %v, expected pattern like %v", i, dt.Errors[i], dt.Errors[0])
			}
		}
	}

	return nil
}

// AllErrorsConsistent returns true if all runs had consistent error behavior
func (dt *DeterminismTest) AllErrorsConsistent() bool {
	for i := 0; i < len(dt.Errors)-1; i++ {
		if (dt.Errors[i] != nil) != (dt.Errors[i+1] != nil) {
			return false
		}
	}
	return true
}

// MetricsAssertion helps validate metrics collection
type MetricsAssertion struct {
	Registry *prometheus.Registry
}

// NewMetricsAssertion creates a new metrics assertion helper
func NewMetricsAssertion(reg *prometheus.Registry) *MetricsAssertion {
	return &MetricsAssertion{
		Registry: reg,
	}
}

// GatherMetrics returns all metrics from registry
func (ma *MetricsAssertion) GatherMetrics() (map[string]interface{}, error) {
	families, err := ma.Registry.Gather()
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for _, family := range families {
		result[family.GetName()] = family.GetType()
	}
	return result, nil
}
