package integration

import (
	"fmt"
	"sync"
)

// ServiceType represents different service types in the system
type ServiceType string

const (
	// ServiceTypeAPI represents the main API service
	ServiceTypeAPI ServiceType = "api"

	// ServiceTypeShield represents the authentication service
	ServiceTypeShield ServiceType = "shield"

	// ServiceTypeRelay represents the real-time collaboration service
	ServiceTypeRelay ServiceType = "relay"

	// ServiceTypeStorm represents the observability platform
	ServiceTypeStorm ServiceType = "storm"

	// ServiceTypeDatabase represents the database service
	ServiceTypeDatabase ServiceType = "database"

	// ServiceTypeCache represents the cache service
	ServiceTypeCache ServiceType = "cache"
)

// ServiceEndpoint defines how to reach a service
type ServiceEndpoint struct {
	Type     ServiceType
	Host     string
	Port     int
	Protocol string // http, https, grpc, ws
	BasePath string
}

// URL returns the full endpoint URL
func (se *ServiceEndpoint) URL() string {
	if se.Protocol == "" {
		se.Protocol = "http"
	}

	url := fmt.Sprintf("%s://%s:%d", se.Protocol, se.Host, se.Port)
	if se.BasePath != "" {
		url += se.BasePath
	}
	return url
}

// ServiceRegistry manages service discovery and routing
type ServiceRegistry struct {
	mu       sync.RWMutex
	services map[ServiceType]*ServiceEndpoint
	aliases  map[string]ServiceType // For looking up by name
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		services: make(map[ServiceType]*ServiceEndpoint),
		aliases:  make(map[string]ServiceType),
	}
}

// Register registers a service endpoint
func (sr *ServiceRegistry) Register(endpoint *ServiceEndpoint) error {
	if endpoint == nil {
		return fmt.Errorf("endpoint cannot be nil")
	}

	if endpoint.Type == "" {
		return fmt.Errorf("service type cannot be empty")
	}

	if endpoint.Host == "" || endpoint.Port == 0 {
		return fmt.Errorf("host and port are required")
	}

	sr.mu.Lock()
	defer sr.mu.Unlock()

	sr.services[endpoint.Type] = endpoint
	sr.aliases[string(endpoint.Type)] = endpoint.Type

	return nil
}

// Unregister removes a service endpoint
func (sr *ServiceRegistry) Unregister(serviceType ServiceType) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if _, exists := sr.services[serviceType]; !exists {
		return fmt.Errorf("service %s not registered", serviceType)
	}

	delete(sr.services, serviceType)
	delete(sr.aliases, string(serviceType))

	return nil
}

// Get retrieves a service endpoint by type
func (sr *ServiceRegistry) Get(serviceType ServiceType) *ServiceEndpoint {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	return sr.services[serviceType]
}

// GetByName retrieves a service endpoint by name string
func (sr *ServiceRegistry) GetByName(name string) *ServiceEndpoint {
	sr.mu.RLock()
	serviceType, exists := sr.aliases[name]
	sr.mu.RUnlock()

	if !exists {
		return nil
	}

	return sr.Get(serviceType)
}

// List returns all registered services
func (sr *ServiceRegistry) List() map[ServiceType]*ServiceEndpoint {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	services := make(map[ServiceType]*ServiceEndpoint, len(sr.services))
	for k, v := range sr.services {
		services[k] = v
	}
	return services
}

// IsRegistered checks if a service is registered
func (sr *ServiceRegistry) IsRegistered(serviceType ServiceType) bool {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	_, exists := sr.services[serviceType]
	return exists
}

// Count returns the number of registered services
func (sr *ServiceRegistry) Count() int {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	return len(sr.services)
}

// Clear removes all registered services
func (sr *ServiceRegistry) Clear() {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	sr.services = make(map[ServiceType]*ServiceEndpoint)
	sr.aliases = make(map[string]ServiceType)
}
