package storm

import (
	"fmt"
	"time"
)

// ErrorType represents different error types
type ErrorType string

const (
	// ErrTypeConnection indicates connection error
	ErrTypeConnection ErrorType = "connection"

	// ErrTypeTimeout indicates timeout error
	ErrTypeTimeout ErrorType = "timeout"

	// ErrTypeUnauthorized indicates authorization error
	ErrTypeUnauthorized ErrorType = "unauthorized"

	// ErrTypeValidation indicates validation error
	ErrTypeValidation ErrorType = "validation"

	// ErrTypeInternal indicates internal server error
	ErrTypeInternal ErrorType = "internal"

	// ErrTypeQueueFull indicates queue is full
	ErrTypeQueueFull ErrorType = "queue_full"

	// ErrTypeNotRunning indicates queue is not running
	ErrTypeNotRunning ErrorType = "not_running"

	// ErrTypeRateLimit indicates rate limit error
	ErrTypeRateLimit ErrorType = "rate_limit"
)

// Error represents a Storm error
type Error struct {
	Type      ErrorType
	Message   string
	Cause     error
	Timestamp time.Time
	Retryable bool
	StatusCode int
}

// NewError creates a new Storm error
func NewError(errType ErrorType, message string, cause error) *Error {
	return &Error{
		Type:      errType,
		Message:   message,
		Cause:     cause,
		Timestamp: time.Now(),
		Retryable: isRetryable(errType),
	}
}

// NewErrorWithStatus creates a new Storm error with HTTP status code
func NewErrorWithStatus(errType ErrorType, message string, cause error, statusCode int) *Error {
	err := NewError(errType, message, cause)
	err.StatusCode = statusCode
	err.Retryable = isRetryableStatus(statusCode)
	return err
}

// Error implements error interface
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
	return e.Cause
}

// IsRetryable returns whether the error is retryable
func (e *Error) IsRetryable() bool {
	return e.Retryable
}

// isRetryable determines if an error type is retryable
func isRetryable(errType ErrorType) bool {
	switch errType {
	case ErrTypeConnection, ErrTypeTimeout, ErrTypeRateLimit:
		return true
	default:
		return false
	}
}

// isRetryableStatus determines if an HTTP status code is retryable
func isRetryableStatus(statusCode int) bool {
	switch statusCode {
	case 429, 502, 503, 504: // Rate limit, Bad Gateway, Service Unavailable, Gateway Timeout
		return true
	default:
		return statusCode >= 500
	}
}

// ConnectionError creates a connection error
func ConnectionError(message string, cause error) *Error {
	return NewError(ErrTypeConnection, message, cause)
}

// TimeoutError creates a timeout error
func TimeoutError(message string) *Error {
	return NewError(ErrTypeTimeout, message, nil)
}

// UnauthorizedError creates an authorization error
func UnauthorizedError(message string) *Error {
	return NewError(ErrTypeUnauthorized, message, nil)
}

// ValidationError creates a validation error
func ValidationError(message string) *Error {
	return NewError(ErrTypeValidation, message, nil)
}

// InternalError creates an internal server error
func InternalError(message string, cause error) *Error {
	return NewErrorWithStatus(ErrTypeInternal, message, cause, 500)
}

// QueueFullError creates a queue full error
func QueueFullError() *Error {
	return NewError(ErrTypeQueueFull, "queue is full", nil)
}

// NotRunningError creates a not running error
func NotRunningError() *Error {
	return NewError(ErrTypeNotRunning, "queue is not running", nil)
}

// RateLimitError creates a rate limit error
func RateLimitError(retryAfter time.Duration) *Error {
	err := NewError(ErrTypeRateLimit, fmt.Sprintf("rate limit exceeded, retry after %v", retryAfter), nil)
	return err
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
}

// DefaultRetryPolicy returns sensible retry defaults
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		BackoffFactor:  2.0,
	}
}

// CalculateBackoff calculates backoff duration for attempt number
func (rp *RetryPolicy) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}

	// Calculate exponential backoff
	backoff := time.Duration(rp.InitialBackoff) * time.Duration(1<<uint(attempt-1))

	// Cap at max backoff
	if backoff > rp.MaxBackoff {
		backoff = rp.MaxBackoff
	}

	return backoff
}

// IsExhausted returns whether retry attempts are exhausted
func (rp *RetryPolicy) IsExhausted(attempt int) bool {
	return attempt >= rp.MaxRetries
}

// ErrorRecovery provides error recovery utilities
type ErrorRecovery struct {
	policy      *RetryPolicy
	lastError   *Error
	attemptCount int
}

// NewErrorRecovery creates a new error recovery handler
func NewErrorRecovery(policy *RetryPolicy) *ErrorRecovery {
	if policy == nil {
		policy = DefaultRetryPolicy()
	}

	return &ErrorRecovery{
		policy: policy,
	}
}

// RecordError records an error
func (er *ErrorRecovery) RecordError(err *Error) bool {
	er.lastError = err
	er.attemptCount++

	// Check if we should retry
	return err.IsRetryable() && !er.policy.IsExhausted(er.attemptCount)
}

// GetBackoffDuration returns backoff duration for next retry
func (er *ErrorRecovery) GetBackoffDuration() time.Duration {
	return er.policy.CalculateBackoff(er.attemptCount)
}

// Reset resets error recovery state
func (er *ErrorRecovery) Reset() {
	er.lastError = nil
	er.attemptCount = 0
}

// GetLastError returns the last recorded error
func (er *ErrorRecovery) GetLastError() *Error {
	return er.lastError
}

// GetAttemptCount returns the number of attempts
func (er *ErrorRecovery) GetAttemptCount() int {
	return er.attemptCount
}
