package observability

import (
	"context"

	"github.com/google/uuid"
)

// contextKey is a private type for context keys to avoid collisions
type contextKey string

const (
	// CorrelationIDKey is the context key for correlation IDs
	CorrelationIDKey contextKey = "correlation_id"
	// RequestIDKey is an alias for correlation ID (common in HTTP contexts)
	RequestIDKey contextKey = "request_id"
)

// WithCorrelationID adds a correlation ID to the context
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

// GetCorrelationID retrieves the correlation ID from context
func GetCorrelationID(ctx context.Context) string {
	if id, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return id
	}
	return ""
}

// WithRequestID adds a request ID to the context (alias for correlation ID)
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	// Fall back to correlation ID
	return GetCorrelationID(ctx)
}

// GenerateCorrelationID generates a new UUID-based correlation ID
func GenerateCorrelationID() string {
	return uuid.New().String()
}

// EnsureCorrelationID ensures a correlation ID exists in the context,
// generating one if necessary
func EnsureCorrelationID(ctx context.Context) (context.Context, string) {
	correlationID := GetCorrelationID(ctx)
	if correlationID == "" {
		correlationID = GenerateCorrelationID()
		ctx = WithCorrelationID(ctx, correlationID)
	}
	return ctx, correlationID
}

// EnsureRequestID ensures a request ID exists in the context,
// generating one if necessary
func EnsureRequestID(ctx context.Context) (context.Context, string) {
	requestID := GetRequestID(ctx)
	if requestID == "" {
		requestID = GenerateCorrelationID()
		ctx = WithRequestID(ctx, requestID)
	}
	return ctx, requestID
}
