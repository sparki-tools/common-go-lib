package observability

import (
	"context"
	"fmt"
)

// PropagationContextKey is a type for context keys used in propagation
type PropagationContextKey string

const (
	// PropCorrelationIDKey is the context key for correlation ID in propagation
	PropCorrelationIDKey PropagationContextKey = "prop_correlation_id"

	// PropSpanIDKey is the context key for span ID in propagation
	PropSpanIDKey PropagationContextKey = "prop_span_id"

	// PropTraceIDKey is the context key for trace ID in propagation
	PropTraceIDKey PropagationContextKey = "prop_trace_id"

	// PropRequestIDKey is the context key for request ID in propagation
	PropRequestIDKey PropagationContextKey = "prop_request_id"

	// PropUserIDKey is the context key for user ID in propagation
	PropUserIDKey PropagationContextKey = "prop_user_id"

	// PropSessionIDKey is the context key for session ID in propagation
	PropSessionIDKey PropagationContextKey = "prop_session_id"

	// PropFrameworkKey is the context key for the observability framework in propagation
	PropFrameworkKey PropagationContextKey = "prop_observability_framework"
)

// ContextPropagation provides utilities for propagating observability context
type ContextPropagation struct {
	framework *Framework
}

// NewContextPropagation creates a new context propagation instance
func NewContextPropagation(f *Framework) *ContextPropagation {
	return &ContextPropagation{framework: f}
}

// ContextWithCorrelationID adds a correlation ID to context
func (cp *ContextPropagation) ContextWithCorrelationID(ctx context.Context, correlationID string) context.Context {
	if correlationID == "" {
		correlationID = generateTraceID()
	}
	return context.WithValue(ctx, PropCorrelationIDKey, correlationID)
}

// ContextWithUserID adds a user ID to context
func (cp *ContextPropagation) ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, PropUserIDKey, userID)
}

// ContextWithSessionID adds a session ID to context
func (cp *ContextPropagation) ContextWithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, PropSessionIDKey, sessionID)
}

// ContextWithRequestID adds a request ID to context
func (cp *ContextPropagation) ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		requestID = generateTraceID()
	}
	return context.WithValue(ctx, PropRequestIDKey, requestID)
}

// ContextWithFramework adds the framework to context
func (cp *ContextPropagation) ContextWithFramework(ctx context.Context) context.Context {
	return context.WithValue(ctx, PropFrameworkKey, cp.framework)
}

// GetCorrelationID extracts correlation ID from context
func (cp *ContextPropagation) GetCorrelationID(ctx context.Context) string {
	if val := ctx.Value(PropCorrelationIDKey); val != nil {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return ""
}

// GetUserID extracts user ID from context
func (cp *ContextPropagation) GetUserID(ctx context.Context) string {
	if val := ctx.Value(PropUserIDKey); val != nil {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return ""
}

// GetSessionID extracts session ID from context
func (cp *ContextPropagation) GetSessionID(ctx context.Context) string {
	if val := ctx.Value(PropSessionIDKey); val != nil {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return ""
}

// GetRequestID extracts request ID from context
func (cp *ContextPropagation) GetRequestID(ctx context.Context) string {
	if val := ctx.Value(PropRequestIDKey); val != nil {
		if id, ok := val.(string); ok {
			return id
		}
	}
	return ""
}

// GetFramework extracts framework from context
func (cp *ContextPropagation) GetFramework(ctx context.Context) *Framework {
	if val := ctx.Value(PropFrameworkKey); val != nil {
		if f, ok := val.(*Framework); ok {
			return f
		}
	}
	return nil
}

// PropagationHeaders represents headers for cross-service context propagation
type PropagationHeaders struct {
	CorrelationID string
	TraceID       string
	SpanID        string
	RequestID     string
	UserID        string
}

// ToPropagationHeaders extracts all observable context and returns as propagation headers
func (cp *ContextPropagation) ToPropagationHeaders(ctx context.Context) *PropagationHeaders {
	headers := &PropagationHeaders{
		CorrelationID: cp.GetCorrelationID(ctx),
		RequestID:     cp.GetRequestID(ctx),
		UserID:        cp.GetUserID(ctx),
	}

	if val := ctx.Value("trace"); val != nil {
		if traceID, ok := val.(string); ok {
			headers.TraceID = traceID
		}
	}

	if val := ctx.Value("span"); val != nil {
		if spanID, ok := val.(string); ok {
			headers.SpanID = spanID
		}
	}

	return headers
}

// FromPropagationHeaders reconstructs context from propagation headers
func (cp *ContextPropagation) FromPropagationHeaders(ctx context.Context, headers *PropagationHeaders) context.Context {
	if headers == nil {
		return ctx
	}

	if headers.CorrelationID != "" {
		ctx = cp.ContextWithCorrelationID(ctx, headers.CorrelationID)
	}

	if headers.RequestID != "" {
		ctx = cp.ContextWithRequestID(ctx, headers.RequestID)
	}

	if headers.UserID != "" {
		ctx = cp.ContextWithUserID(ctx, headers.UserID)
	}

	if headers.TraceID != "" {
		ctx = context.WithValue(ctx, "trace", headers.TraceID)
	}

	if headers.SpanID != "" {
		ctx = context.WithValue(ctx, "span", headers.SpanID)
	}

	return ctx
}

// FormatContextInfo formats context information for logging
func (cp *ContextPropagation) FormatContextInfo(ctx context.Context) string {
	correlationID := cp.GetCorrelationID(ctx)
	userID := cp.GetUserID(ctx)
	requestID := cp.GetRequestID(ctx)

	info := fmt.Sprintf("correlation_id=%s", correlationID)
	if requestID != "" {
		info += fmt.Sprintf(", request_id=%s", requestID)
	}
	if userID != "" {
		info += fmt.Sprintf(", user_id=%s", userID)
	}

	return info
}

// PropagationMap converts context to a map suitable for JSON marshaling
func (cp *ContextPropagation) PropagationMap(ctx context.Context) map[string]interface{} {
	return map[string]interface{}{
		"correlation_id": cp.GetCorrelationID(ctx),
		"request_id":     cp.GetRequestID(ctx),
		"user_id":        cp.GetUserID(ctx),
		"session_id":     cp.GetSessionID(ctx),
	}
}

// ChildContext creates a child context with a new child span
func (cp *ContextPropagation) ChildContext(ctx context.Context, operationName string) (context.Context, *SpanContext) {
	// Create child span (this will inherit parent trace ID)
	return cp.framework.StartSpan(ctx, operationName)
}

// EndChildSpan ends a child span
func (cp *ContextPropagation) EndChildSpan(ctx context.Context, span *SpanContext, err error) {
	cp.framework.EndSpan(ctx, span, err)
}

// WithRequestContext sets up a complete request context with all observability data
func (cp *ContextPropagation) WithRequestContext(ctx context.Context, correlationID, userID, sessionID string) context.Context {
	ctx = cp.ContextWithCorrelationID(ctx, correlationID)
	ctx = cp.ContextWithRequestID(ctx, correlationID)
	if userID != "" {
		ctx = cp.ContextWithUserID(ctx, userID)
	}
	if sessionID != "" {
		ctx = cp.ContextWithSessionID(ctx, sessionID)
	}
	ctx = cp.ContextWithFramework(ctx)
	return ctx
}
