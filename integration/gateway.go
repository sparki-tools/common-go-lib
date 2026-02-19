package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Request represents a standardized inter-service request
type Request struct {
	Service  ServiceType            `json:"service"`
	Method   string                 `json:"method"`
	Path     string                 `json:"path"`
	Body     json.RawMessage        `json:"body,omitempty"`
	Headers  map[string]string      `json:"headers,omitempty"`
	Query    map[string]string      `json:"query,omitempty"`
	Timeout  time.Duration          `json:"timeout,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Response represents a standardized inter-service response
type Response struct {
	Status    int                    `json:"status"`
	Body      json.RawMessage        `json:"body"`
	Headers   map[string][]string    `json:"headers,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Duration  time.Duration          `json:"duration"`
	Timestamp int64                  `json:"timestamp"`
}

// Gateway handles inter-service communication
type Gateway struct {
	registry *ServiceRegistry
	factory  *ClientFactory
	logger   *zap.Logger
}

// NewGateway creates a new service gateway
func NewGateway(registry *ServiceRegistry, factory *ClientFactory) (*Gateway, error) {
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}

	if factory == nil {
		return nil, fmt.Errorf("factory cannot be nil")
	}

	logger := factory.logger
	if logger == nil {
		logger, _ = zap.NewProduction()
	}

	return &Gateway{
		registry: registry,
		factory:  factory,
		logger:   logger,
	}, nil
}

// Call executes an inter-service request
func (g *Gateway) Call(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	if req.Service == "" {
		return nil, fmt.Errorf("service type is required")
	}

	if req.Method == "" {
		req.Method = "GET"
	}

	if req.Timeout == 0 {
		req.Timeout = 30 * time.Second
	}

	// Create context with timeout
	callCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	// Create HTTP request
	httpReq, err := g.factory.CreateRequest(req.Service, req.Method, g.buildPath(req))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add request body if provided
	if len(req.Body) > 0 {
		httpReq.Body = io.NopCloser(bytes.NewReader(req.Body))
		httpReq.ContentLength = int64(len(req.Body))
	}

	// Add custom headers
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	// Add correlation ID from context if available
	if correlationID, ok := ctx.Value("correlation_id").(string); ok {
		httpReq.Header.Set("X-Correlation-ID", correlationID)
	}

	// Execute request
	start := time.Now()
	httpResp, err := g.factory.Do(req.Service, httpReq.WithContext(callCtx))
	duration := time.Since(start)

	if err != nil {
		g.logger.Error("inter-service call failed",
			zap.String("service", string(req.Service)),
			zap.String("method", req.Method),
			zap.String("path", req.Path),
			zap.Duration("duration", duration),
			zap.Error(err),
		)
		return nil, fmt.Errorf("inter-service call failed: %w", err)
	}

	defer httpResp.Body.Close()

	// Read response body
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Build response
	resp := &Response{
		Status:    httpResp.StatusCode,
		Body:      body,
		Headers:   httpResp.Header,
		Duration:  duration,
		Timestamp: time.Now().Unix(),
	}

	// Check for error status
	if httpResp.StatusCode >= 400 {
		resp.Error = fmt.Sprintf("HTTP %d", httpResp.StatusCode)
		g.logger.Warn("inter-service call error response",
			zap.String("service", string(req.Service)),
			zap.Int("status", httpResp.StatusCode),
			zap.Duration("duration", duration),
		)
	} else {
		g.logger.Debug("inter-service call succeeded",
			zap.String("service", string(req.Service)),
			zap.String("method", req.Method),
			zap.String("path", req.Path),
			zap.Int("status", httpResp.StatusCode),
			zap.Duration("duration", duration),
		)
	}

	return resp, nil
}

// buildPath builds the request path with query parameters
func (g *Gateway) buildPath(req *Request) string {
	path := req.Path
	if path == "" {
		path = "/"
	}

	// Add query parameters
	if len(req.Query) > 0 {
		first := true
		for key, value := range req.Query {
			if first {
				path += "?"
				first = false
			} else {
				path += "&"
			}
			path += fmt.Sprintf("%s=%s", key, value)
		}
	}

	return path
}

// CallJSON is a convenience method for JSON requests and responses
func (g *Gateway) CallJSON(ctx context.Context, service ServiceType, method, path string, requestBody interface{}, responseBody interface{}) error {
	// Marshal request body
	var body json.RawMessage
	if requestBody != nil {
		b, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = b
	}

	// Create and execute request
	req := &Request{
		Service: service,
		Method:  method,
		Path:    path,
		Body:    body,
	}

	resp, err := g.Call(ctx, req)
	if err != nil {
		return err
	}

	// Check response status
	if resp.Status >= 400 {
		return fmt.Errorf("service returned error: %s", resp.Error)
	}

	// Unmarshal response body
	if responseBody != nil && len(resp.Body) > 0 {
		if err := json.Unmarshal(resp.Body, responseBody); err != nil {
			return fmt.Errorf("failed to unmarshal response body: %w", err)
		}
	}

	return nil
}

// Health checks the health of a service
func (g *Gateway) Health(ctx context.Context, service ServiceType) (bool, error) {
	req := &Request{
		Service: service,
		Method:  "GET",
		Path:    "/health",
		Timeout: 5 * time.Second,
	}

	resp, err := g.Call(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.Status == http.StatusOK, nil
}

// Ready checks if a service is ready
func (g *Gateway) Ready(ctx context.Context, service ServiceType) (bool, error) {
	req := &Request{
		Service: service,
		Method:  "GET",
		Path:    "/ready",
		Timeout: 5 * time.Second,
	}

	resp, err := g.Call(ctx, req)
	if err != nil {
		return false, err
	}

	return resp.Status == http.StatusOK, nil
}

// Close closes the gateway
func (g *Gateway) Close() error {
	return g.factory.Close()
}
