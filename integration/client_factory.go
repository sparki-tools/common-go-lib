package integration

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ClientConfig defines configuration for service clients
type ClientConfig struct {
	// Timeout for HTTP requests
	Timeout time.Duration

	// Retry configuration
	MaxRetries int
	RetryDelay time.Duration

	// Authentication
	APIKey string
	Token  string

	// TLS configuration
	InsecureSkipVerify bool

	// Custom headers
	Headers map[string]string

	// Logger
	Logger *zap.Logger
}

// DefaultClientConfig returns sensible defaults
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Timeout:    30 * time.Second,
		MaxRetries: 3,
		RetryDelay: 100 * time.Millisecond,
		Headers:    make(map[string]string),
	}
}

// ClientFactory creates unified HTTP clients for services
type ClientFactory struct {
	registry *ServiceRegistry
	config   *ClientConfig
	logger   *zap.Logger

	mu      sync.Mutex // guards clients; Gateway calls arrive concurrently
	clients map[ServiceType]*http.Client
}

// NewClientFactory creates a new client factory
func NewClientFactory(registry *ServiceRegistry, config *ClientConfig) (*ClientFactory, error) {
	if registry == nil {
		return nil, fmt.Errorf("registry cannot be nil")
	}

	if config == nil {
		config = DefaultClientConfig()
	}

	if config.Logger == nil {
		logger, _ := zap.NewProduction()
		config.Logger = logger
	}

	return &ClientFactory{
		registry: registry,
		config:   config,
		logger:   config.Logger,
		clients:  make(map[ServiceType]*http.Client),
	}, nil
}

// GetClient returns an HTTP client for a service
func (cf *ClientFactory) GetClient(serviceType ServiceType) (*http.Client, error) {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	// Return cached client if available
	if client, exists := cf.clients[serviceType]; exists {
		return client, nil
	}

	// Get service endpoint
	endpoint := cf.registry.Get(serviceType)
	if endpoint == nil {
		return nil, fmt.Errorf("service %s not registered", serviceType)
	}

	// Create new client with appropriate configuration
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   cf.config.Timeout,
	}

	// Cache the client
	cf.clients[serviceType] = client

	cf.logger.Debug("http client created",
		zap.String("service", string(serviceType)),
		zap.String("endpoint", endpoint.URL()),
	)

	return client, nil
}

// CreateRequest creates a request with standard headers and authentication
func (cf *ClientFactory) CreateRequest(serviceType ServiceType, method, path string) (*http.Request, error) {
	endpoint := cf.registry.Get(serviceType)
	if endpoint == nil {
		return nil, fmt.Errorf("service %s not registered", serviceType)
	}

	url := endpoint.URL() + path
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add standard headers
	req.Header.Set("User-Agent", "sparki-integration-client/1.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	// Add custom headers
	for key, value := range cf.config.Headers {
		req.Header.Set(key, value)
	}

	// Add authentication
	if cf.config.APIKey != "" {
		req.Header.Set("X-API-Key", cf.config.APIKey)
	}
	if cf.config.Token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cf.config.Token))
	}

	return req, nil
}

// Do executes an HTTP request with retry logic
func (cf *ClientFactory) Do(serviceType ServiceType, req *http.Request) (*http.Response, error) {
	client, err := cf.GetClient(serviceType)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt <= cf.config.MaxRetries; attempt++ {
		resp, err := client.Do(req)

		if err != nil {
			lastErr = err
			if attempt < cf.config.MaxRetries {
				time.Sleep(cf.config.RetryDelay * time.Duration(attempt+1))
				continue
			}
			return nil, fmt.Errorf("request failed after %d retries: %w", cf.config.MaxRetries, err)
		}

		// Retry on 5xx errors or 429 (rate limit)
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			resp.Body.Close()
			if attempt < cf.config.MaxRetries {
				time.Sleep(cf.config.RetryDelay * time.Duration(attempt+1))
				continue
			}
		}

		return resp, nil
	}

	return nil, lastErr
}

// Close closes all cached clients
func (cf *ClientFactory) Close() error {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	for serviceType, client := range cf.clients {
		if client != nil && client.Transport != nil {
			if transport, ok := client.Transport.(*http.Transport); ok {
				transport.CloseIdleConnections()
			}
		}
		cf.logger.Debug("client closed", zap.String("service", string(serviceType)))
	}

	cf.clients = make(map[ServiceType]*http.Client)
	return nil
}
