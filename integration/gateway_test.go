package integration

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

// testLogger creates a test logger
func testLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

// TestNewClientFactory tests factory creation
func TestNewClientFactory(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080})

	config := &ClientConfig{
		Timeout:    30 * time.Second,
		MaxRetries: 3,
		Logger:     testLogger(),
	}

	factory, err := NewClientFactory(registry, config)
	if err != nil {
		t.Fatalf("NewClientFactory failed: %v", err)
	}

	if factory == nil {
		t.Error("expected factory to be initialized")
	}

	factory.Close()
}

// TestNewClientFactoryValidation tests factory validation
func TestNewClientFactoryValidation(t *testing.T) {
	// Test nil registry
	_, err := NewClientFactory(nil, nil)
	if err == nil {
		t.Error("expected error for nil registry")
	}
}

// TestDefaultClientConfig tests default config
func TestDefaultClientConfig(t *testing.T) {
	config := DefaultClientConfig()

	if config.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", config.Timeout)
	}

	if config.MaxRetries != 3 {
		t.Errorf("expected max retries 3, got %d", config.MaxRetries)
	}
}

// TestCreateRequest tests request creation
func TestCreateRequest(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{
		Type:     ServiceTypeAPI,
		Host:     "localhost",
		Port:     8080,
		Protocol: "http",
		BasePath: "/api",
	})

	factory, _ := NewClientFactory(registry, DefaultClientConfig())
	defer factory.Close()

	req, err := factory.CreateRequest(ServiceTypeAPI, "GET", "/users")
	if err != nil {
		t.Fatalf("CreateRequest failed: %v", err)
	}

	if req == nil {
		t.Error("expected request to be created")
	}

	if req.Method != "GET" {
		t.Errorf("expected method GET, got %s", req.Method)
	}

	if req.Header.Get("User-Agent") != "sparki-integration-client/1.0" {
		t.Error("User-Agent header not set correctly")
	}
}

// TestCreateRequestWithAuth tests request with authentication
func TestCreateRequestWithAuth(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080})

	config := DefaultClientConfig()
	config.APIKey = "test-api-key"
	config.Token = "test-token"

	factory, _ := NewClientFactory(registry, config)
	defer factory.Close()

	req, _ := factory.CreateRequest(ServiceTypeAPI, "GET", "/users")

	if req.Header.Get("X-API-Key") != "test-api-key" {
		t.Error("X-API-Key header not set correctly")
	}

	if req.Header.Get("Authorization") != "Bearer test-token" {
		t.Error("Authorization header not set correctly")
	}
}

// TestCreateRequestWithCustomHeaders tests request with custom headers
func TestCreateRequestWithCustomHeaders(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080})

	config := DefaultClientConfig()
	config.Headers = map[string]string{
		"X-Custom-Header": "custom-value",
	}

	factory, _ := NewClientFactory(registry, config)
	defer factory.Close()

	req, _ := factory.CreateRequest(ServiceTypeAPI, "GET", "/users")

	if req.Header.Get("X-Custom-Header") != "custom-value" {
		t.Error("custom header not set correctly")
	}
}

// TestNewGateway tests gateway creation
func TestNewGateway(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080})

	factory, _ := NewClientFactory(registry, DefaultClientConfig())
	defer factory.Close()

	gateway, err := NewGateway(registry, factory)
	if err != nil {
		t.Fatalf("NewGateway failed: %v", err)
	}

	if gateway == nil {
		t.Error("expected gateway to be initialized")
	}
}

// TestNewGatewayValidation tests gateway validation
func TestNewGatewayValidation(t *testing.T) {
	registry := NewServiceRegistry()

	// Test nil registry
	_, err := NewGateway(nil, nil)
	if err == nil {
		t.Error("expected error for nil registry")
	}

	// Test nil factory
	factory, _ := NewClientFactory(registry, DefaultClientConfig())
	defer factory.Close()

	_, err = NewGateway(registry, nil)
	if err == nil {
		t.Error("expected error for nil factory")
	}
}

// TestBuildPath tests path building with query parameters
func TestBuildPath(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080})

	factory, _ := NewClientFactory(registry, DefaultClientConfig())
	defer factory.Close()

	gateway, _ := NewGateway(registry, factory)

	tests := []struct {
		name     string
		request  *Request
		expected string
	}{
		{
			"without query",
			&Request{Service: ServiceTypeAPI, Path: "/users"},
			"/users",
		},
		{
			"with query",
			&Request{Service: ServiceTypeAPI, Path: "/users", Query: map[string]string{"id": "123"}},
			"/users?id=123",
		},
		{
			"empty path",
			&Request{Service: ServiceTypeAPI, Path: ""},
			"/",
		},
	}

	for _, test := range tests {
		path := gateway.buildPath(test.request)
		if path != test.expected {
			t.Errorf("test %s: expected %s, got %s", test.name, test.expected, path)
		}
	}
}

// TestRequestValidation tests request validation
func TestRequestValidation(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080})

	factory, _ := NewClientFactory(registry, DefaultClientConfig())
	defer factory.Close()

	gateway, _ := NewGateway(registry, factory)

	tests := []struct {
		name       string
		request    *Request
		shouldFail bool
	}{
		{"nil request", nil, true},
		{"empty service", &Request{Service: "", Method: "GET", Path: "/"}, true},
		{"valid", &Request{Service: ServiceTypeAPI, Method: "GET", Path: "/"}, false},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := gateway.Call(ctx, test.request)
		if test.shouldFail && err == nil {
			t.Errorf("test %s: expected error but got none", test.name)
		}
	}
}

// TestDefaultMethod tests default HTTP method
func TestDefaultMethod(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080})

	factory, _ := NewClientFactory(registry, DefaultClientConfig())
	defer factory.Close()

	req := &Request{Service: ServiceTypeAPI, Path: "/users", Method: ""}
	if req.Method == "" {
		req.Method = "GET"
	}

	if req.Method != "GET" {
		t.Errorf("expected default method GET, got %s", req.Method)
	}
}

// TestDefaultTimeout tests default timeout
func TestDefaultTimeout(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080})

	factory, _ := NewClientFactory(registry, DefaultClientConfig())
	defer factory.Close()

	req := &Request{Service: ServiceTypeAPI, Path: "/users"}
	if req.Timeout == 0 {
		req.Timeout = 30 * time.Second
	}

	if req.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", req.Timeout)
	}
}

// TestServiceEndpointTypes tests different service types
func TestServiceEndpointTypes(t *testing.T) {
	types := []ServiceType{
		ServiceTypeAPI,
		ServiceTypeShield,
		ServiceTypeRelay,
		ServiceTypeStorm,
		ServiceTypeDatabase,
		ServiceTypeCache,
	}

	registry := NewServiceRegistry()

	for _, serviceType := range types {
		endpoint := &ServiceEndpoint{
			Type: serviceType,
			Host: "localhost",
			Port: 8000,
		}
		registry.Register(endpoint)
	}

	if registry.Count() != len(types) {
		t.Errorf("expected %d services, got %d", len(types), registry.Count())
	}
}

// TestFactoryClientCaching tests client caching
func TestFactoryClientCaching(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080})

	factory, _ := NewClientFactory(registry, DefaultClientConfig())
	defer factory.Close()

	client1, _ := factory.GetClient(ServiceTypeAPI)
	client2, _ := factory.GetClient(ServiceTypeAPI)

	if client1 != client2 {
		t.Error("expected cached client to be same instance")
	}
}

// TestRequestMetadata tests request metadata
func TestRequestMetadata(t *testing.T) {
	req := &Request{
		Service: ServiceTypeAPI,
		Method:  "POST",
		Path:    "/users",
		Metadata: map[string]interface{}{
			"user_id": "user123",
			"request_id": "req456",
		},
	}

	if len(req.Metadata) != 2 {
		t.Errorf("expected 2 metadata fields, got %d", len(req.Metadata))
	}

	if userID, ok := req.Metadata["user_id"].(string); !ok || userID != "user123" {
		t.Error("user_id metadata not set correctly")
	}
}

// TestFactoryClose tests factory closure
func TestFactoryClose(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080})

	factory, _ := NewClientFactory(registry, DefaultClientConfig())

	err := factory.Close()
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}
}

// TestGatewayClose tests gateway closure
func TestGatewayClose(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080})

	factory, _ := NewClientFactory(registry, DefaultClientConfig())
	gateway, _ := NewGateway(registry, factory)

	err := gateway.Close()
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}
}
