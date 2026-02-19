package integration

import (
	"testing"
)

// TestNewServiceRegistry tests registry creation
func TestNewServiceRegistry(t *testing.T) {
	registry := NewServiceRegistry()

	if registry == nil {
		t.Error("expected registry to be initialized")
	}

	if registry.Count() != 0 {
		t.Errorf("expected 0 services, got %d", registry.Count())
	}
}

// TestRegisterService tests service registration
func TestRegisterService(t *testing.T) {
	registry := NewServiceRegistry()

	endpoint := &ServiceEndpoint{
		Type:     ServiceTypeAPI,
		Host:     "localhost",
		Port:     8080,
		Protocol: "http",
	}

	err := registry.Register(endpoint)
	if err != nil {
		t.Errorf("Register failed: %v", err)
	}

	if !registry.IsRegistered(ServiceTypeAPI) {
		t.Error("service should be registered")
	}

	if registry.Count() != 1 {
		t.Errorf("expected 1 service, got %d", registry.Count())
	}
}

// TestRegisterMultipleServices tests registering multiple services
func TestRegisterMultipleServices(t *testing.T) {
	registry := NewServiceRegistry()

	endpoints := []*ServiceEndpoint{
		{Type: ServiceTypeAPI, Host: "localhost", Port: 8080},
		{Type: ServiceTypeShield, Host: "localhost", Port: 8000},
		{Type: ServiceTypeRelay, Host: "localhost", Port: 8081},
	}

	for _, endpoint := range endpoints {
		err := registry.Register(endpoint)
		if err != nil {
			t.Errorf("Register failed: %v", err)
		}
	}

	if registry.Count() != 3 {
		t.Errorf("expected 3 services, got %d", registry.Count())
	}
}

// TestRegisterValidation tests registration validation
func TestRegisterValidation(t *testing.T) {
	registry := NewServiceRegistry()

	tests := []struct {
		name    string
		endpoint *ServiceEndpoint
		shouldFail bool
	}{
		{"nil endpoint", nil, true},
		{"empty type", &ServiceEndpoint{Type: "", Host: "localhost", Port: 8080}, true},
		{"empty host", &ServiceEndpoint{Type: ServiceTypeAPI, Host: "", Port: 8080}, true},
		{"zero port", &ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 0}, true},
		{"valid", &ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080}, false},
	}

	for _, test := range tests {
		err := registry.Register(test.endpoint)
		if test.shouldFail && err == nil {
			t.Errorf("test %s: expected error but got none", test.name)
		}
		if !test.shouldFail && err != nil {
			t.Errorf("test %s: unexpected error: %v", test.name, err)
		}
	}
}

// TestGetService tests retrieving a service
func TestGetService(t *testing.T) {
	registry := NewServiceRegistry()

	endpoint := &ServiceEndpoint{
		Type:     ServiceTypeAPI,
		Host:     "localhost",
		Port:     8080,
		Protocol: "http",
		BasePath: "/api",
	}

	registry.Register(endpoint)

	retrieved := registry.Get(ServiceTypeAPI)
	if retrieved == nil {
		t.Error("expected to retrieve service")
	}

	if retrieved.Type != ServiceTypeAPI {
		t.Errorf("expected type %s, got %s", ServiceTypeAPI, retrieved.Type)
	}

	if retrieved.Port != 8080 {
		t.Errorf("expected port 8080, got %d", retrieved.Port)
	}
}

// TestGetNonexistentService tests retrieving non-existent service
func TestGetNonexistentService(t *testing.T) {
	registry := NewServiceRegistry()

	retrieved := registry.Get(ServiceTypeAPI)
	if retrieved != nil {
		t.Error("expected nil for non-existent service")
	}
}

// TestUnregisterService tests unregistering a service
func TestUnregisterService(t *testing.T) {
	registry := NewServiceRegistry()

	endpoint := &ServiceEndpoint{
		Type: ServiceTypeAPI,
		Host: "localhost",
		Port: 8080,
	}

	registry.Register(endpoint)
	if registry.Count() != 1 {
		t.Error("service should be registered")
	}

	err := registry.Unregister(ServiceTypeAPI)
	if err != nil {
		t.Errorf("Unregister failed: %v", err)
	}

	if registry.Count() != 0 {
		t.Error("service should be unregistered")
	}
}

// TestUnregisterNonexistent tests unregistering non-existent service
func TestUnregisterNonexistent(t *testing.T) {
	registry := NewServiceRegistry()

	err := registry.Unregister(ServiceTypeAPI)
	if err == nil {
		t.Error("expected error when unregistering non-existent service")
	}
}

// TestGetByName tests retrieving service by name
func TestGetByName(t *testing.T) {
	registry := NewServiceRegistry()

	endpoint := &ServiceEndpoint{
		Type: ServiceTypeAPI,
		Host: "localhost",
		Port: 8080,
	}

	registry.Register(endpoint)

	retrieved := registry.GetByName("api")
	if retrieved == nil {
		t.Error("expected to retrieve service by name")
	}

	if retrieved.Type != ServiceTypeAPI {
		t.Error("retrieved service has wrong type")
	}
}

// TestServiceEndpointURL tests URL generation
func TestServiceEndpointURL(t *testing.T) {
	tests := []struct {
		name     string
		endpoint *ServiceEndpoint
		expected string
	}{
		{
			"with base path",
			&ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080, Protocol: "http", BasePath: "/api"},
			"http://localhost:8080/api",
		},
		{
			"without base path",
			&ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080, Protocol: "https"},
			"https://localhost:8080",
		},
		{
			"default protocol",
			&ServiceEndpoint{Type: ServiceTypeAPI, Host: "api.example.com", Port: 443},
			"http://api.example.com:443",
		},
	}

	for _, test := range tests {
		url := test.endpoint.URL()
		if url != test.expected {
			t.Errorf("test %s: expected %s, got %s", test.name, test.expected, url)
		}
	}
}

// TestListServices tests listing all services
func TestListServices(t *testing.T) {
	registry := NewServiceRegistry()

	endpoints := []*ServiceEndpoint{
		{Type: ServiceTypeAPI, Host: "localhost", Port: 8080},
		{Type: ServiceTypeShield, Host: "localhost", Port: 8000},
	}

	for _, endpoint := range endpoints {
		registry.Register(endpoint)
	}

	services := registry.List()
	if len(services) != 2 {
		t.Errorf("expected 2 services, got %d", len(services))
	}

	if _, exists := services[ServiceTypeAPI]; !exists {
		t.Error("API service not in list")
	}

	if _, exists := services[ServiceTypeShield]; !exists {
		t.Error("Shield service not in list")
	}
}

// TestClearRegistry tests clearing all services
func TestClearRegistry(t *testing.T) {
	registry := NewServiceRegistry()

	// Register services
	for i := 0; i < 3; i++ {
		registry.Register(&ServiceEndpoint{
			Type: ServiceTypeAPI,
			Host: "localhost",
			Port: 8080 + i,
		})
	}

	if registry.Count() == 0 {
		t.Error("registry should not be empty")
	}

	registry.Clear()

	if registry.Count() != 0 {
		t.Errorf("registry should be empty after clear, got %d", registry.Count())
	}
}

// TestIsRegistered tests service registration check
func TestIsRegistered(t *testing.T) {
	registry := NewServiceRegistry()

	if registry.IsRegistered(ServiceTypeAPI) {
		t.Error("service should not be registered initially")
	}

	registry.Register(&ServiceEndpoint{
		Type: ServiceTypeAPI,
		Host: "localhost",
		Port: 8080,
	})

	if !registry.IsRegistered(ServiceTypeAPI) {
		t.Error("service should be registered")
	}
}

// TestDuplicateRegistration tests re-registering a service
func TestDuplicateRegistration(t *testing.T) {
	registry := NewServiceRegistry()

	endpoint1 := &ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 8080}
	endpoint2 := &ServiceEndpoint{Type: ServiceTypeAPI, Host: "localhost", Port: 9090}

	registry.Register(endpoint1)
	registry.Register(endpoint2)

	// Latest registration should override
	retrieved := registry.Get(ServiceTypeAPI)
	if retrieved.Port != 9090 {
		t.Errorf("expected port 9090, got %d", retrieved.Port)
	}
}
