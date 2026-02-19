package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// HealthStatus represents the overall health status
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
)

// ComponentHealth represents the health of a single component
type ComponentHealth struct {
	Status    HealthStatus           `json:"status"`
	Message   string                 `json:"message,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// HealthResponse represents the complete health check response
type HealthResponse struct {
	Status     HealthStatus                `json:"status"`
	Version    string                      `json:"version"`
	Timestamp  time.Time                   `json:"timestamp"`
	Uptime     time.Duration               `json:"uptime"`
	Components map[string]ComponentHealth `json:"components"`
	System     SystemInfo                  `json:"system"`
}

// SystemInfo contains system-level information
type SystemInfo struct {
	GoVersion      string `json:"go_version"`
	NumGoroutines  int    `json:"num_goroutines"`
	NumCPU         int    `json:"num_cpu"`
	MemAllocMB     uint64 `json:"mem_alloc_mb"`
	MemSysMB       uint64 `json:"mem_sys_mb"`
	GCPauseLastMS  uint64 `json:"gc_pause_last_ms"`
	NumGC          uint32 `json:"num_gc"`
}

// HealthChecker is a function that checks the health of a component
type HealthChecker func(ctx context.Context) ComponentHealth

// HealthManager manages health checks for all components
type HealthManager struct {
	checkers  map[string]HealthChecker
	startTime time.Time
	version   string
	mu        sync.RWMutex
}

// NewHealthManager creates a new health manager
func NewHealthManager(version string) *HealthManager {
	return &HealthManager{
		checkers:  make(map[string]HealthChecker),
		startTime: time.Now(),
		version:   version,
	}
}

// RegisterChecker registers a health checker for a component
func (hm *HealthManager) RegisterChecker(name string, checker HealthChecker) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.checkers[name] = checker
}

// Check performs health checks on all registered components
func (hm *HealthManager) Check(ctx context.Context) HealthResponse {
	hm.mu.RLock()
	checkers := make(map[string]HealthChecker, len(hm.checkers))
	for k, v := range hm.checkers {
		checkers[k] = v
	}
	hm.mu.RUnlock()

	// Execute all health checks concurrently
	components := make(map[string]ComponentHealth)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, checker := range checkers {
		wg.Add(1)
		go func(n string, c HealthChecker) {
			defer wg.Done()

			// Set timeout for each health check
			checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			health := c(checkCtx)

			mu.Lock()
			components[n] = health
			mu.Unlock()
		}(name, checker)
	}

	wg.Wait()

	// Determine overall status
	overallStatus := StatusHealthy
	for _, comp := range components {
		if comp.Status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
			break
		}
		if comp.Status == StatusDegraded && overallStatus == StatusHealthy {
			overallStatus = StatusDegraded
		}
	}

	return HealthResponse{
		Status:     overallStatus,
		Version:    hm.version,
		Timestamp:  time.Now(),
		Uptime:     time.Since(hm.startTime),
		Components: components,
		System:     hm.getSystemInfo(),
	}
}

// getSystemInfo collects system-level metrics
func (hm *HealthManager) getSystemInfo() SystemInfo {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return SystemInfo{
		GoVersion:      runtime.Version(),
		NumGoroutines:  runtime.NumGoroutine(),
		NumCPU:         runtime.NumCPU(),
		MemAllocMB:     m.Alloc / 1024 / 1024,
		MemSysMB:       m.Sys / 1024 / 1024,
		GCPauseLastMS:  m.PauseNs[(m.NumGC+255)%256] / 1000000,
		NumGC:          m.NumGC,
	}
}

// FiberHandler returns a Fiber handler for health checks
func (hm *HealthManager) FiberHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		health := hm.Check(c.Context())

		statusCode := http.StatusOK
		if health.Status == StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		} else if health.Status == StatusDegraded {
			statusCode = http.StatusOK // Still return 200 but indicate degraded
		}

		return c.Status(statusCode).JSON(health)
	}
}

// FiberReadyHandler returns a Fiber handler for readiness checks
// (simpler check that only verifies critical components)
func (hm *HealthManager) FiberReadyHandler(criticalComponents []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		health := hm.Check(c.Context())

		// Check only critical components
		for _, compName := range criticalComponents {
			if comp, ok := health.Components[compName]; ok {
				if comp.Status == StatusUnhealthy {
					return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
						"ready":   false,
						"reason":  fmt.Sprintf("component %s is unhealthy", compName),
						"details": comp,
					})
				}
			}
		}

		return c.Status(http.StatusOK).JSON(fiber.Map{
			"ready":     true,
			"timestamp": time.Now(),
		})
	}
}

// StandardHTTPHandler returns a standard http.Handler for health checks
func (hm *HealthManager) StandardHTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		health := hm.Check(r.Context())

		statusCode := http.StatusOK
		if health.Status == StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(health)
	})
}

// Predefined health checkers

// DatabaseHealthChecker creates a health checker for database connectivity
func DatabaseHealthChecker(pingFunc func(context.Context) error) HealthChecker {
	return func(ctx context.Context) ComponentHealth {
		start := time.Now()
		err := pingFunc(ctx)
		duration := time.Since(start)

		if err != nil {
			return ComponentHealth{
				Status:    StatusUnhealthy,
				Message:   "Database connection failed",
				Timestamp: time.Now(),
				Error:     err.Error(),
			}
		}

		return ComponentHealth{
			Status:    StatusHealthy,
			Message:   "Database connection successful",
			Timestamp: time.Now(),
			Details: map[string]interface{}{
				"ping_duration_ms": duration.Milliseconds(),
			},
		}
	}
}

// CacheHealthChecker creates a health checker for cache connectivity
func CacheHealthChecker(pingFunc func(context.Context) error) HealthChecker {
	return func(ctx context.Context) ComponentHealth {
		start := time.Now()
		err := pingFunc(ctx)
		duration := time.Since(start)

		if err != nil {
			return ComponentHealth{
				Status:    StatusUnhealthy,
				Message:   "Cache connection failed",
				Timestamp: time.Now(),
				Error:     err.Error(),
			}
		}

		return ComponentHealth{
			Status:    StatusHealthy,
			Message:   "Cache connection successful",
			Timestamp: time.Now(),
			Details: map[string]interface{}{
				"ping_duration_ms": duration.Milliseconds(),
			},
		}
	}
}

// StaticHealthChecker creates a simple always-healthy checker
func StaticHealthChecker(name string) HealthChecker {
	return func(ctx context.Context) ComponentHealth {
		return ComponentHealth{
			Status:    StatusHealthy,
			Message:   fmt.Sprintf("%s is operational", name),
			Timestamp: time.Now(),
		}
	}
}

// ThresholdHealthChecker creates a health checker that checks a value against thresholds
func ThresholdHealthChecker(name string, getValue func() float64, warnThreshold, criticalThreshold float64) HealthChecker {
	return func(ctx context.Context) ComponentHealth {
		value := getValue()

		var status HealthStatus
		var message string

		if value >= criticalThreshold {
			status = StatusUnhealthy
			message = fmt.Sprintf("%s is critical (%.2f >= %.2f)", name, value, criticalThreshold)
		} else if value >= warnThreshold {
			status = StatusDegraded
			message = fmt.Sprintf("%s is degraded (%.2f >= %.2f)", name, value, warnThreshold)
		} else {
			status = StatusHealthy
			message = fmt.Sprintf("%s is healthy (%.2f)", name, value)
		}

		return ComponentHealth{
			Status:    status,
			Message:   message,
			Timestamp: time.Now(),
			Details: map[string]interface{}{
				"value":              value,
				"warn_threshold":     warnThreshold,
				"critical_threshold": criticalThreshold,
			},
		}
	}
}
