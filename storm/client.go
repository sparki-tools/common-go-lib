package storm

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

// Client provides unified interface to Storm observability platform
type Client struct {
	endpoint string
	client   *http.Client
	logger   *zap.Logger
	apiKey   string
	timeout  time.Duration
}

// ClientConfig configures Storm client
type ClientConfig struct {
	Endpoint string
	APIKey   string
	Timeout  time.Duration
	Logger   *zap.Logger
}

// NewClient creates a new Storm client
func NewClient(config *ClientConfig) (*Client, error) {
	if config == nil {
		config = &ClientConfig{
			Endpoint: "http://localhost:8080",
			Timeout:  30 * time.Second,
		}
	}

	if config.Endpoint == "" {
		config.Endpoint = "http://localhost:8080"
	}

	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	if config.Logger == nil {
		logger, _ := zap.NewProduction()
		config.Logger = logger
	}

	return &Client{
		endpoint: config.Endpoint,
		apiKey:   config.APIKey,
		timeout:  config.Timeout,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		logger: config.Logger,
	}, nil
}

// MetricsPayload represents metrics to send to Storm
type MetricsPayload struct {
	ServiceName string                 `json:"service_name"`
	Timestamp   int64                  `json:"timestamp"`
	Metrics     map[string]interface{} `json:"metrics"`
	Tags        map[string]string      `json:"tags"`
}

// TracePayload represents trace data to send to Storm
type TracePayload struct {
	TraceID      string                 `json:"trace_id"`
	SpanID       string                 `json:"span_id"`
	ParentSpanID string                 `json:"parent_span_id,omitempty"`
	Operation    string                 `json:"operation"`
	Duration     int64                  `json:"duration_ms"`
	Status       string                 `json:"status"`
	ServiceName  string                 `json:"service_name"`
	Timestamp    int64                  `json:"timestamp"`
	Tags         map[string]interface{} `json:"tags"`
}

// LogPayload represents log data to send to Storm
type LogPayload struct {
	Level       string                 `json:"level"`
	Message     string                 `json:"message"`
	ServiceName string                 `json:"service_name"`
	Timestamp   int64                  `json:"timestamp"`
	Fields      map[string]interface{} `json:"fields"`
}

// Response represents Storm API response
type Response struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// PublishMetrics sends metrics to Storm
func (c *Client) PublishMetrics(ctx context.Context, metrics *MetricsPayload) error {
	if metrics == nil {
		return fmt.Errorf("metrics payload cannot be nil")
	}

	if metrics.Timestamp == 0 {
		metrics.Timestamp = time.Now().Unix()
	}

	payload, err := json.Marshal(metrics)
	if err != nil {
		c.logger.Error("failed to marshal metrics", zap.Error(err))
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/v1/metrics", c.endpoint), bytes.NewBuffer(payload))
	if err != nil {
		c.logger.Error("failed to create request", zap.Error(err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Error("failed to send metrics", zap.Error(err))
		return fmt.Errorf("failed to send metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		c.logger.Error("metrics publish failed", zap.Int("status", resp.StatusCode), zap.String("body", string(body)))
		return fmt.Errorf("metrics publish failed: status %d", resp.StatusCode)
	}

	c.logger.Debug("metrics published successfully", zap.String("service", metrics.ServiceName))
	return nil
}

// PublishTrace sends trace data to Storm
func (c *Client) PublishTrace(ctx context.Context, trace *TracePayload) error {
	if trace == nil {
		return fmt.Errorf("trace payload cannot be nil")
	}

	if trace.Timestamp == 0 {
		trace.Timestamp = time.Now().Unix()
	}

	payload, err := json.Marshal(trace)
	if err != nil {
		c.logger.Error("failed to marshal trace", zap.Error(err))
		return fmt.Errorf("failed to marshal trace: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/v1/traces", c.endpoint), bytes.NewBuffer(payload))
	if err != nil {
		c.logger.Error("failed to create request", zap.Error(err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Error("failed to send trace", zap.Error(err))
		return fmt.Errorf("failed to send trace: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		c.logger.Error("trace publish failed", zap.Int("status", resp.StatusCode), zap.String("body", string(body)))
		return fmt.Errorf("trace publish failed: status %d", resp.StatusCode)
	}

	c.logger.Debug("trace published successfully", zap.String("trace_id", trace.TraceID))
	return nil
}

// PublishLog sends log data to Storm
func (c *Client) PublishLog(ctx context.Context, log *LogPayload) error {
	if log == nil {
		return fmt.Errorf("log payload cannot be nil")
	}

	if log.Timestamp == 0 {
		log.Timestamp = time.Now().Unix()
	}

	payload, err := json.Marshal(log)
	if err != nil {
		c.logger.Error("failed to marshal log", zap.Error(err))
		return fmt.Errorf("failed to marshal log: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/api/v1/logs", c.endpoint), bytes.NewBuffer(payload))
	if err != nil {
		c.logger.Error("failed to create request", zap.Error(err))
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Error("failed to send log", zap.Error(err))
		return fmt.Errorf("failed to send log: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		c.logger.Error("log publish failed", zap.Int("status", resp.StatusCode), zap.String("body", string(body)))
		return fmt.Errorf("log publish failed: status %d", resp.StatusCode)
	}

	c.logger.Debug("log published successfully", zap.String("level", log.Level))
	return nil
}

// Health checks Storm service health
func (c *Client) Health(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/health", c.endpoint), nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Error("health check failed", zap.Error(err))
		return false, fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// Close closes the Storm client
func (c *Client) Close() error {
	c.logger.Info("Storm client closed")
	return nil
}
