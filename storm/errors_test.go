package storm

import (
	"errors"
	"testing"
	"time"
)

// TestErrorTypes tests error type creation
func TestErrorTypes(t *testing.T) {
	tests := []struct {
		name     string
		errType  ErrorType
		message  string
		expected bool
	}{
		{"connection", ErrTypeConnection, "connection refused", true},
		{"timeout", ErrTypeTimeout, "request timeout", true},
		{"unauthorized", ErrTypeUnauthorized, "invalid api key", false},
		{"validation", ErrTypeValidation, "invalid payload", false},
		{"internal", ErrTypeInternal, "internal error", false},
	}

	for _, test := range tests {
		err := NewError(test.errType, test.message, nil)

		if err.Type != test.errType {
			t.Errorf("test %s: expected type %s, got %s", test.name, test.errType, err.Type)
		}

		if err.Message != test.message {
			t.Errorf("test %s: expected message %s, got %s", test.name, test.message, err.Message)
		}

		if err.IsRetryable() != test.expected {
			t.Errorf("test %s: expected retryable %v, got %v", test.name, test.expected, err.IsRetryable())
		}
	}
}

// TestNewError tests error creation
func TestNewError(t *testing.T) {
	cause := errors.New("network unreachable")
	err := NewError(ErrTypeConnection, "failed to connect", cause)

	if err.Type != ErrTypeConnection {
		t.Error("error type not set correctly")
	}

	if err.Message != "failed to connect" {
		t.Error("error message not set correctly")
	}

	if err.Cause != cause {
		t.Error("error cause not set correctly")
	}

	if err.Timestamp.IsZero() {
		t.Error("error timestamp should be set")
	}
}

// TestErrorString tests error string representation
func TestErrorString(t *testing.T) {
	cause := errors.New("network error")
	err := NewError(ErrTypeConnection, "connection failed", cause)

	errStr := err.Error()
	if errStr == "" {
		t.Error("error string should not be empty")
	}

	if !contains(errStr, "connection") {
		t.Error("error string should contain error type")
	}

	if !contains(errStr, "connection failed") {
		t.Error("error string should contain message")
	}
}

// TestErrorUnwrap tests error unwrap
func TestErrorUnwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := NewError(ErrTypeInternal, "wrapped error", cause)

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Error("unwrap should return original cause")
	}
}

// TestConnectionError tests connection error helper
func TestConnectionError(t *testing.T) {
	err := ConnectionError("connection refused", errors.New("ECONNREFUSED"))

	if err.Type != ErrTypeConnection {
		t.Error("error type should be connection")
	}

	if !err.IsRetryable() {
		t.Error("connection error should be retryable")
	}
}

// TestTimeoutError tests timeout error helper
func TestTimeoutError(t *testing.T) {
	err := TimeoutError("request timed out after 30s")

	if err.Type != ErrTypeTimeout {
		t.Error("error type should be timeout")
	}

	if !err.IsRetryable() {
		t.Error("timeout error should be retryable")
	}
}

// TestUnauthorizedError tests unauthorized error helper
func TestUnauthorizedError(t *testing.T) {
	err := UnauthorizedError("invalid api key")

	if err.Type != ErrTypeUnauthorized {
		t.Error("error type should be unauthorized")
	}

	if err.IsRetryable() {
		t.Error("unauthorized error should not be retryable")
	}
}

// TestValidationError tests validation error helper
func TestValidationError(t *testing.T) {
	err := ValidationError("missing required field: service_name")

	if err.Type != ErrTypeValidation {
		t.Error("error type should be validation")
	}

	if err.IsRetryable() {
		t.Error("validation error should not be retryable")
	}
}

// TestRetryPolicy tests retry policy
func TestRetryPolicy(t *testing.T) {
	policy := &RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		BackoffFactor:  2.0,
	}

	if policy.MaxRetries != 3 {
		t.Errorf("expected max retries 3, got %d", policy.MaxRetries)
	}

	if policy.InitialBackoff != 100*time.Millisecond {
		t.Errorf("expected initial backoff 100ms, got %v", policy.InitialBackoff)
	}
}

// TestDefaultRetryPolicy tests default retry policy
func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	if policy.MaxRetries != 3 {
		t.Errorf("expected max retries 3, got %d", policy.MaxRetries)
	}

	if policy.MaxBackoff != 10*time.Second {
		t.Errorf("expected max backoff 10s, got %v", policy.MaxBackoff)
	}
}

// TestCalculateBackoff tests backoff calculation
func TestCalculateBackoff(t *testing.T) {
	policy := DefaultRetryPolicy()

	// First attempt: 100ms
	backoff1 := policy.CalculateBackoff(1)
	if backoff1 != 100*time.Millisecond {
		t.Errorf("expected backoff 100ms, got %v", backoff1)
	}

	// Second attempt: 200ms (100 * 2^1)
	backoff2 := policy.CalculateBackoff(2)
	if backoff2 != 200*time.Millisecond {
		t.Errorf("expected backoff 200ms, got %v", backoff2)
	}

	// Third attempt: 400ms (100 * 2^2)
	backoff3 := policy.CalculateBackoff(3)
	if backoff3 != 400*time.Millisecond {
		t.Errorf("expected backoff 400ms, got %v", backoff3)
	}
}

// TestBackoffMaxCap tests backoff maximum cap
func TestBackoffMaxCap(t *testing.T) {
	policy := DefaultRetryPolicy()

	// High attempt number should be capped at max backoff
	backoff := policy.CalculateBackoff(10)
	if backoff > policy.MaxBackoff {
		t.Errorf("backoff %v should not exceed max %v", backoff, policy.MaxBackoff)
	}
}

// TestIsExhausted tests retry exhaustion check
func TestIsExhausted(t *testing.T) {
	policy := &RetryPolicy{MaxRetries: 3}

	if policy.IsExhausted(2) {
		t.Error("should not be exhausted at attempt 2")
	}

	if !policy.IsExhausted(3) {
		t.Error("should be exhausted at attempt 3")
	}

	if !policy.IsExhausted(4) {
		t.Error("should be exhausted at attempt 4")
	}
}

// TestErrorRecovery tests error recovery handler
func TestErrorRecovery(t *testing.T) {
	policy := DefaultRetryPolicy()
	recovery := NewErrorRecovery(policy)

	// Record first error
	err1 := ConnectionError("network unreachable", nil)
	shouldRetry1 := recovery.RecordError(err1)

	if !shouldRetry1 {
		t.Error("should retry after first connection error")
	}

	if recovery.GetAttemptCount() != 1 {
		t.Errorf("expected attempt count 1, got %d", recovery.GetAttemptCount())
	}

	// Record second error
	err2 := ConnectionError("network unreachable", nil)
	shouldRetry2 := recovery.RecordError(err2)

	if !shouldRetry2 {
		t.Error("should retry after second connection error")
	}

	if recovery.GetAttemptCount() != 2 {
		t.Errorf("expected attempt count 2, got %d", recovery.GetAttemptCount())
	}
}

// TestErrorRecoveryExhaustion tests error recovery exhaustion
func TestErrorRecoveryExhaustion(t *testing.T) {
	policy := &RetryPolicy{MaxRetries: 2}
	recovery := NewErrorRecovery(policy)

	// Record errors until exhausted
	for i := 0; i < 3; i++ {
		err := ConnectionError("network unreachable", nil)
		shouldRetry := recovery.RecordError(err)

		if i < 1 && !shouldRetry {
			t.Errorf("should retry at attempt %d", i+1)
		}

		if i >= 2 && shouldRetry {
			t.Errorf("should not retry at attempt %d (exhausted)", i+1)
		}
	}
}

// TestErrorRecoveryReset tests error recovery reset
func TestErrorRecoveryReset(t *testing.T) {
	recovery := NewErrorRecovery(nil)

	err := ConnectionError("test", nil)
	recovery.RecordError(err)

	if recovery.GetAttemptCount() != 1 {
		t.Error("attempt count should be 1")
	}

	recovery.Reset()

	if recovery.GetAttemptCount() != 0 {
		t.Error("attempt count should be 0 after reset")
	}

	if recovery.GetLastError() != nil {
		t.Error("last error should be nil after reset")
	}
}

// TestQueueFullError tests queue full error
func TestQueueFullError(t *testing.T) {
	err := QueueFullError()

	if err.Type != ErrTypeQueueFull {
		t.Error("error type should be queue full")
	}

	if err.IsRetryable() {
		t.Error("queue full error should not be retryable")
	}
}

// TestRateLimitError tests rate limit error
func TestRateLimitError(t *testing.T) {
	err := RateLimitError(5 * time.Second)

	if err.Type != ErrTypeRateLimit {
		t.Error("error type should be rate limit")
	}

	if !err.IsRetryable() {
		t.Error("rate limit error should be retryable")
	}
}

// TestNewErrorWithStatus tests error with HTTP status
func TestNewErrorWithStatus(t *testing.T) {
	err := NewErrorWithStatus(ErrTypeInternal, "server error", errors.New("db connection failed"), 500)

	if err.StatusCode != 500 {
		t.Errorf("expected status code 500, got %d", err.StatusCode)
	}

	if !err.IsRetryable() {
		t.Error("5xx errors should be retryable")
	}
}

// Helper function to check string contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
