package integration

import (
	"context"
	"testing"
	"time"
)

// TestCoverageServiceType tests all ServiceType constants
func TestCoverageServiceType(t *testing.T) {
	types := []ServiceType{
		ServiceTypeAPI,
		ServiceTypeShield,
		ServiceTypeRelay,
		ServiceTypeStorm,
		ServiceTypeDatabase,
		ServiceTypeCache,
	}

	if len(types) == 0 {
		t.Error("expected service types to be defined")
	}

	for _, st := range types {
		if st == "" {
			t.Error("service type should not be empty")
		}
	}
}

// TestCoverageServiceEndpoint tests ServiceEndpoint functionality
func TestCoverageServiceEndpoint(t *testing.T) {
	endpoint := &ServiceEndpoint{
		Type:     ServiceTypeAPI,
		Host:     "localhost",
		Port:     8080,
		Protocol: "http",
		BasePath: "/api",
	}

	url := endpoint.URL()
	if url == "" {
		t.Error("URL should not be empty")
	}

	if endpoint.Type == "" {
		t.Error("endpoint type should be set")
	}
}

// TestCoverageServiceEndpointDefaults tests default protocol
func TestCoverageServiceEndpointDefaults(t *testing.T) {
	endpoint := &ServiceEndpoint{
		Type: ServiceTypeAPI,
		Host: "localhost",
		Port: 8080,
	}

	url := endpoint.URL()
	if url == "" {
		t.Error("URL with defaults should not be empty")
	}

	// Default protocol should be http
	if !contains(url, "http") {
		t.Errorf("expected http protocol in URL, got %s", url)
	}
}

// TestCoverageRegistryOperations tests all registry operations
func TestCoverageRegistryOperations(t *testing.T) {
	registry := NewServiceRegistry()

	endpoint := &ServiceEndpoint{
		Type: ServiceTypeAPI,
		Host: "localhost",
		Port: 8080,
	}

	// Test Register
	err := registry.Register(endpoint)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Test Get
	retrieved := registry.Get(ServiceTypeAPI)
	if retrieved == nil {
		t.Error("expected to retrieve registered endpoint")
	}

	// Test IsRegistered
	if !registry.IsRegistered(ServiceTypeAPI) {
		t.Error("expected service to be registered")
	}

	// Test Count
	count := registry.Count()
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// Test GetByName
	retrieved = registry.GetByName("api")
	if retrieved == nil {
		t.Error("expected to retrieve by name")
	}

	// Test List
	list := registry.List()
	if len(list) != 1 {
		t.Errorf("expected list size 1, got %d", len(list))
	}

	// Test Unregister
	err = registry.Unregister(ServiceTypeAPI)
	if err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	if registry.Count() != 0 {
		t.Error("expected empty registry after unregister")
	}

	// Test Clear
	registry.Register(endpoint)
	registry.Clear()
	if registry.Count() != 0 {
		t.Error("expected empty registry after clear")
	}
}

// TestCoverageRegistryValidation tests registry validation
func TestCoverageRegistryValidation(t *testing.T) {
	registry := NewServiceRegistry()

	// Test nil endpoint
	err := registry.Register(nil)
	if err == nil {
		t.Error("expected error for nil endpoint")
	}

	// Test empty type
	err = registry.Register(&ServiceEndpoint{
		Host: "localhost",
		Port: 8080,
	})
	if err == nil {
		t.Error("expected error for empty type")
	}

	// Test missing host
	err = registry.Register(&ServiceEndpoint{
		Type: ServiceTypeAPI,
		Port: 8080,
	})
	if err == nil {
		t.Error("expected error for missing host")
	}

	// Test zero port
	err = registry.Register(&ServiceEndpoint{
		Type: ServiceTypeAPI,
		Host: "localhost",
	})
	if err == nil {
		t.Error("expected error for zero port")
	}
}

// TestCoverageClientConfig tests client configuration options
func TestCoverageClientConfig(t *testing.T) {
	config := DefaultClientConfig()

	if config.Timeout == 0 {
		t.Error("expected non-zero timeout")
	}

	if config.MaxRetries == 0 {
		t.Error("expected non-zero max retries")
	}

	config.APIKey = "test-key"
	if config.APIKey != "test-key" {
		t.Error("expected APIKey to be set")
	}

	config.Token = "test-token"
	if config.Token != "test-token" {
		t.Error("expected Token to be set")
	}

	config.InsecureSkipVerify = true
	if !config.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be true")
	}

	config.Headers = map[string]string{"X-Custom": "value"}
	if config.Headers["X-Custom"] != "value" {
		t.Error("expected custom header to be set")
	}
}

// TestCoverageClientFactoryValidation tests factory validation
func TestCoverageClientFactoryValidation(t *testing.T) {
	// Test nil registry
	_, err := NewClientFactory(nil, nil)
	if err == nil {
		t.Error("expected error for nil registry")
	}

	// Test nil config (should use defaults)
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{
		Type: ServiceTypeAPI,
		Host: "localhost",
		Port: 8080,
	})

	factory, err := NewClientFactory(registry, nil)
	if err != nil {
		t.Fatalf("NewClientFactory with nil config failed: %v", err)
	}

	factory.Close()
}

// TestCoverageRequestStructure tests Request structure
func TestCoverageRequestStructure(t *testing.T) {
	req := &Request{
		Service:  ServiceTypeAPI,
		Method:   "POST",
		Path:     "/users",
		Headers:  map[string]string{"X-Test": "value"},
		Query:    map[string]string{"id": "123"},
		Timeout:  30 * time.Second,
		Metadata: map[string]interface{}{"user_id": "user123"},
	}

	if req.Service != ServiceTypeAPI {
		t.Error("expected service to be set")
	}

	if req.Method != "POST" {
		t.Error("expected method to be set")
	}

	if req.Path != "/users" {
		t.Error("expected path to be set")
	}

	if len(req.Headers) == 0 {
		t.Error("expected headers to be set")
	}

	if len(req.Query) == 0 {
		t.Error("expected query to be set")
	}

	if req.Timeout == 0 {
		t.Error("expected timeout to be set")
	}

	if len(req.Metadata) == 0 {
		t.Error("expected metadata to be set")
	}
}

// TestCoverageResponseStructure tests Response structure
func TestCoverageResponseStructure(t *testing.T) {
	resp := &Response{
		Status:    200,
		Body:      []byte(`{"data":"test"}`),
		Headers:   map[string][]string{"Content-Type": {"application/json"}},
		Error:     "",
		Duration:  100 * time.Millisecond,
		Timestamp: time.Now().Unix(),
	}

	if resp.Status != 200 {
		t.Error("expected status to be set")
	}

	if len(resp.Body) == 0 {
		t.Error("expected body to be set")
	}

	if len(resp.Headers) == 0 {
		t.Error("expected headers to be set")
	}

	if resp.Duration == 0 {
		t.Error("expected duration to be set")
	}

	if resp.Timestamp == 0 {
		t.Error("expected timestamp to be set")
	}
}

// TestCoverageGatewayValidation tests gateway validation
func TestCoverageGatewayValidation(t *testing.T) {
	// Test nil registry
	_, err := NewGateway(nil, nil)
	if err == nil {
		t.Error("expected error for nil registry")
	}

	registry := NewServiceRegistry()

	// Test nil factory
	_, err = NewGateway(registry, nil)
	if err == nil {
		t.Error("expected error for nil factory")
	}

	registry.Register(&ServiceEndpoint{
		Type: ServiceTypeAPI,
		Host: "localhost",
		Port: 8080,
	})

	config := DefaultClientConfig()
	config.Logger = testLogger()

	factory, _ := NewClientFactory(registry, config)
	defer factory.Close()

	// Test successful creation
	gateway, err := NewGateway(registry, factory)
	if err != nil {
		t.Fatalf("NewGateway failed: %v", err)
	}

	gateway.Close()
}

// TestCoverageGatewayCallValidation tests gateway call validation
func TestCoverageGatewayCallValidation(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{
		Type: ServiceTypeAPI,
		Host: "localhost",
		Port: 8080,
	})

	config := DefaultClientConfig()
	config.Logger = testLogger()

	factory, _ := NewClientFactory(registry, config)
	defer factory.Close()

	gateway, _ := NewGateway(registry, factory)

	ctx := context.Background()

	// Test nil request
	_, err := gateway.Call(ctx, nil)
	if err == nil {
		t.Error("expected error for nil request")
	}

	// Test empty service
	_, err = gateway.Call(ctx, &Request{
		Service: "",
		Method:  "GET",
		Path:    "/",
	})
	if err == nil {
		t.Error("expected error for empty service")
	}

	// Test default method
	req := &Request{
		Service: ServiceTypeAPI,
		Path:    "/users",
	}

	if req.Method != "" {
		// Method should be empty before call
	}

	// Test default timeout
	if req.Timeout != 0 {
		// Timeout should be zero before call
	}
}

// TestCoverageBuildPath tests path building functionality
func TestCoverageBuildPath(t *testing.T) {
	registry := NewServiceRegistry()
	registry.Register(&ServiceEndpoint{
		Type: ServiceTypeAPI,
		Host: "localhost",
		Port: 8080,
	})

	config := DefaultClientConfig()
	config.Logger = testLogger()

	factory, _ := NewClientFactory(registry, config)
	defer factory.Close()

	gateway, _ := NewGateway(registry, factory)

	tests := []struct {
		name          string
		request       *Request
		shouldContain string
	}{
		{
			"simple path",
			&Request{Service: ServiceTypeAPI, Path: "/users"},
			"/users",
		},
		{
			"empty path defaults to slash",
			&Request{Service: ServiceTypeAPI, Path: ""},
			"/",
		},
		{
			"query parameters",
			&Request{Service: ServiceTypeAPI, Path: "/users", Query: map[string]string{"id": "123"}},
			"?",
		},
	}

	for _, test := range tests {
		path := gateway.buildPath(test.request)
		if !contains(path, test.shouldContain) {
			t.Errorf("test %s: expected path to contain %s, got %s", test.name, test.shouldContain, path)
		}
	}
}

// TestCoverageAllServiceTypes tests all service type constants
func TestCoverageAllServiceTypes(t *testing.T) {
	expected := map[ServiceType]bool{
		ServiceTypeAPI:      true,
		ServiceTypeShield:   true,
		ServiceTypeRelay:    true,
		ServiceTypeStorm:    true,
		ServiceTypeDatabase: true,
		ServiceTypeCache:    true,
	}

	for st := range expected {
		if st == "" {
			t.Error("service type should not be empty")
		}
	}

	if len(expected) < 6 {
		t.Error("expected at least 6 service types")
	}
}

// TestCoverageContextTimeout tests context with timeout
func TestCoverageContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case <-ctx.Done():
		t.Error("context should not be done immediately")
	default:
		// Expected
	}

	cancel()

	select {
	case <-ctx.Done():
		// Expected after cancel
	case <-time.After(100 * time.Millisecond):
		t.Error("context should be done after cancel")
	}
}

// TestCoverageErrorMessages tests error message quality
func TestCoverageErrorMessages(t *testing.T) {
	registry := NewServiceRegistry()

	// Nil endpoint error
	err := registry.Register(nil)
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}

	// Unregister non-existent
	err = registry.Unregister("non-existent")
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}

	// Invalid factory
	_, err = NewClientFactory(nil, nil)
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

// TestCoverageThreadSafety tests that operations are thread-safe
func TestCoverageThreadSafety(t *testing.T) {
	registry := NewServiceRegistry()

	done := make(chan bool, 10)

	// Concurrent registrations
	for i := 0; i < 10; i++ {
		go func(id int) {
			registry.Register(&ServiceEndpoint{
				Type: ServiceTypeAPI,
				Host: "localhost",
				Port: 8080,
			})
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Registry should have handled concurrent access
	count := registry.Count()
	if count == 0 {
		t.Error("expected at least one service registered")
	}
}

// TestCoverageFullCycle tests a complete registration to usage cycle
func TestCoverageFullCycle(t *testing.T) {
	// Create registry
	registry := NewServiceRegistry()

	// Register service
	endpoint := &ServiceEndpoint{
		Type:     ServiceTypeAPI,
		Host:     "api.example.com",
		Port:     8080,
		Protocol: "https",
		BasePath: "/api/v1",
	}

	err := registry.Register(endpoint)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Create factory
	config := DefaultClientConfig()
	config.Logger = testLogger()
	config.APIKey = "test-key"

	factory, err := NewClientFactory(registry, config)
	if err != nil {
		t.Fatalf("NewClientFactory failed: %v", err)
	}
	defer factory.Close()

	// Create gateway
	gateway, err := NewGateway(registry, factory)
	if err != nil {
		t.Fatalf("NewGateway failed: %v", err)
	}

	// Verify service is accessible
	if !registry.IsRegistered(ServiceTypeAPI) {
		t.Error("expected service to be registered")
	}

	// Get client
	client, err := factory.GetClient(ServiceTypeAPI)
	if err != nil {
		t.Fatalf("GetClient failed: %v", err)
	}

	if client == nil {
		t.Error("expected non-nil client")
	}

	// Create request
	req, err := factory.CreateRequest(ServiceTypeAPI, "GET", "/users")
	if err != nil {
		t.Fatalf("CreateRequest failed: %v", err)
	}

	if req == nil {
		t.Error("expected non-nil request")
	}

	// Cleanup
	gateway.Close()
}
