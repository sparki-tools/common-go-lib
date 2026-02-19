package observability

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for Sparki
type Metrics struct {
	// API Metrics
	HTTPRequestsTotal      *prometheus.CounterVec
	HTTPRequestDuration    *prometheus.HistogramVec
	HTTPRequestSize        *prometheus.HistogramVec
	HTTPResponseSize       *prometheus.HistogramVec
	HTTPResponseStatusCode *prometheus.CounterVec

	// Database Metrics
	DBQueriesTotal    *prometheus.CounterVec
	DBQueryDuration   *prometheus.HistogramVec
	DBConnectionsOpen prometheus.Gauge
	DBConnectionsIdle prometheus.Gauge

	// Cache Metrics
	CacheHitsTotal   *prometheus.CounterVec
	CacheMissesTotal *prometheus.CounterVec
	CacheLatency     *prometheus.HistogramVec

	// Build Metrics
	BuildsTotal       *prometheus.CounterVec
	BuildDuration     *prometheus.HistogramVec
	BuildQueueDepth   prometheus.Gauge
	BuildConcurrent   prometheus.Gauge

	// Deployment Metrics
	DeploymentsTotal    *prometheus.CounterVec
	DeploymentDuration  *prometheus.HistogramVec
	DeploymentSuccessRate *prometheus.GaugeVec
	RollbacksTotal      *prometheus.CounterVec

	// Framework Detection Metrics
	FrameworkDetectionsTotal *prometheus.CounterVec
	FrameworkDetectDuration  *prometheus.HistogramVec

	// Pipeline Metrics
	PipelineRunsTotal    *prometheus.CounterVec
	PipelineRunDuration  *prometheus.HistogramVec
	PipelineStepsTotal   *prometheus.CounterVec

	// Subscription Metrics
	ActiveSubscriptions *prometheus.GaugeVec
	BillingEventsTotal  *prometheus.CounterVec

	// WebSocket Metrics
	WebSocketConnectionsActive prometheus.Gauge
	WebSocketMessagesTotal     *prometheus.CounterVec

	// Error Metrics
	ErrorsTotal *prometheus.CounterVec
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics(namespace string) *Metrics {
	m := &Metrics{
		// API Metrics
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "path", "status_code"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request latency in seconds",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
			},
			[]string{"method", "path"},
		),
		HTTPRequestSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_request_size_bytes",
				Help:      "HTTP request size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 7), // 100B to 100MB
			},
			[]string{"method", "path"},
		),
		HTTPResponseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "http_response_size_bytes",
				Help:      "HTTP response size in bytes",
				Buckets:   prometheus.ExponentialBuckets(100, 10, 7),
			},
			[]string{"method", "path"},
		),
		HTTPResponseStatusCode: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "http_response_status_code_total",
				Help:      "Total HTTP responses by status code",
			},
			[]string{"status_code", "method", "path"},
		),

		// Database Metrics
		DBQueriesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "database_queries_total",
				Help:      "Total database queries executed",
			},
			[]string{"query_type", "table", "status"},
		),
		DBQueryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "database_query_duration_seconds",
				Help:      "Database query latency in seconds",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
			},
			[]string{"query_type", "table"},
		),
		DBConnectionsOpen: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "database_connections_open",
				Help:      "Number of open database connections",
			},
		),
		DBConnectionsIdle: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "database_connections_idle",
				Help:      "Number of idle database connections",
			},
		),

		// Cache Metrics
		CacheHitsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_hits_total",
				Help:      "Total cache hits",
			},
			[]string{"cache_key_type"},
		),
		CacheMissesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "cache_misses_total",
				Help:      "Total cache misses",
			},
			[]string{"cache_key_type"},
		),
		CacheLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "cache_latency_seconds",
				Help:      "Cache operation latency in seconds",
				Buckets:   []float64{0.0001, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1},
			},
			[]string{"operation", "cache_key_type"},
		),

		// Build Metrics
		BuildsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "builds_total",
				Help:      "Total builds executed",
			},
			[]string{"project", "status", "framework"},
		),
		BuildDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "build_duration_seconds",
				Help:      "Build duration in seconds",
				Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600, 1200, 1800},
			},
			[]string{"project", "framework"},
		),
		BuildQueueDepth: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_queue_depth",
				Help:      "Current build queue depth",
			},
		),
		BuildConcurrent: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "build_concurrent",
				Help:      "Current number of concurrent builds",
			},
		),

		// Deployment Metrics
		DeploymentsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "deployments_total",
				Help:      "Total deployments executed",
			},
			[]string{"project", "environment", "status", "platform"},
		),
		DeploymentDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "deployment_duration_seconds",
				Help:      "Deployment duration in seconds",
				Buckets:   []float64{5, 10, 20, 30, 45, 60, 90, 120, 180, 300},
			},
			[]string{"project", "environment", "platform"},
		),
		DeploymentSuccessRate: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "deployment_success_rate",
				Help:      "Deployment success rate (0.0 to 1.0)",
			},
			[]string{"project", "environment"},
		),
		RollbacksTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "rollbacks_total",
				Help:      "Total deployment rollbacks",
			},
			[]string{"project", "environment", "reason"},
		),

		// Framework Detection Metrics
		FrameworkDetectionsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "framework_detections_total",
				Help:      "Total framework detections",
			},
			[]string{"framework", "language", "status"},
		),
		FrameworkDetectDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "framework_detect_duration_seconds",
				Help:      "Framework detection duration in seconds",
				Buckets:   []float64{0.1, 0.25, 0.5, 1.0, 2.0, 5.0, 10.0},
			},
			[]string{"framework", "language"},
		),

		// Pipeline Metrics
		PipelineRunsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "pipeline_runs_total",
				Help:      "Total pipeline runs",
			},
			[]string{"project", "pipeline", "status", "trigger"},
		),
		PipelineRunDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "pipeline_run_duration_seconds",
				Help:      "Pipeline run duration in seconds",
				Buckets:   []float64{5, 10, 30, 60, 120, 300, 600, 1200, 1800, 3600},
			},
			[]string{"project", "pipeline"},
		),
		PipelineStepsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "pipeline_steps_total",
				Help:      "Total pipeline steps executed",
			},
			[]string{"project", "pipeline", "step_type", "status"},
		),

		// Subscription Metrics
		ActiveSubscriptions: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "subscriptions_active",
				Help:      "Number of active subscriptions",
			},
			[]string{"tier"},
		),
		BillingEventsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "billing_events_total",
				Help:      "Total billing events processed",
			},
			[]string{"event_type", "status"},
		),

		// WebSocket Metrics
		WebSocketConnectionsActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "websocket_connections_active",
				Help:      "Number of active WebSocket connections",
			},
		),
		WebSocketMessagesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "websocket_messages_total",
				Help:      "Total WebSocket messages sent/received",
			},
			[]string{"direction", "message_type"},
		),

		// Error Metrics
		ErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "errors_total",
				Help:      "Total errors by type and severity",
			},
			[]string{"type", "severity", "component"},
		),
	}

	return m
}

// RecordHTTPRequest records an HTTP request metric
func (m *Metrics) RecordHTTPRequest(method, path string, statusCode int, duration float64) {
	statusCodeStr := fmt.Sprintf("%d", statusCode)
	m.HTTPRequestsTotal.WithLabelValues(method, path, statusCodeStr).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
	m.HTTPResponseStatusCode.WithLabelValues(statusCodeStr, method, path).Inc()
}

// RecordCacheHit records a cache hit
func (m *Metrics) RecordCacheHit(keyType string) {
	m.CacheHitsTotal.WithLabelValues(keyType).Inc()
}

// RecordCacheMiss records a cache miss
func (m *Metrics) RecordCacheMiss(keyType string) {
	m.CacheMissesTotal.WithLabelValues(keyType).Inc()
}

// RecordBuild records a build execution
func (m *Metrics) RecordBuild(project, status, framework string, duration float64) {
	m.BuildsTotal.WithLabelValues(project, status, framework).Inc()
	if status == "success" || status == "failed" {
		m.BuildDuration.WithLabelValues(project, framework).Observe(duration)
	}
}

// RecordDeployment records a deployment execution
func (m *Metrics) RecordDeployment(project, environment, status, platform string, duration float64) {
	m.DeploymentsTotal.WithLabelValues(project, environment, status, platform).Inc()
	if status == "success" || status == "failed" {
		m.DeploymentDuration.WithLabelValues(project, environment, platform).Observe(duration)
	}
}

// RecordRollback records a deployment rollback
func (m *Metrics) RecordRollback(project, environment, reason string) {
	m.RollbacksTotal.WithLabelValues(project, environment, reason).Inc()
}

// RecordError records an error
func (m *Metrics) RecordError(errorType, severity, component string) {
	m.ErrorsTotal.WithLabelValues(errorType, severity, component).Inc()
}

// RecordProjectCreated records a project creation
func (m *Metrics) RecordProjectCreated(workspaceID string) {
	// This would typically increment a project counter
	// For now, we can use a general counter or add one if needed
}

// RecordFrameworkDetected records a framework detection
func (m *Metrics) RecordFrameworkDetected(language string, duration float64) {
	m.FrameworkDetectionsTotal.WithLabelValues("unknown", language, "success").Inc()
	m.FrameworkDetectDuration.WithLabelValues("unknown", language).Observe(duration)
}

// RecordPipelineCreated records a pipeline creation
func (m *Metrics) RecordPipelineCreated(pipelineName string) {
	// This would typically increment a pipeline counter
	// For now, we can use a general counter or add one if needed
}

// Handler returns a Fiber-compatible Prometheus metrics handler
func (m *Metrics) Handler() func(c interface{}) error {
	return func(c interface{}) error {
		// This is a placeholder - in production, use promhttp.Handler()
		// with proper Fiber integration
		return nil
	}
}
