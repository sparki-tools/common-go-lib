package observability

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// MiddlewareConfig configures observability middleware behavior
type MiddlewareConfig struct {
	// Skip paths from instrumentation (e.g., health checks, metrics endpoints)
	SkipPaths map[string]bool

	// Track response body (may impact performance)
	TrackResponseBody bool

	// Request ID header name
	RequestIDHeader string

	// Sample rate for profiling (0.0 - 1.0)
	SampleRate float64
}

// DefaultMiddlewareConfig returns sensible defaults
func DefaultMiddlewareConfig() *MiddlewareConfig {
	return &MiddlewareConfig{
		SkipPaths: map[string]bool{
			"/health":  true,
			"/ready":   true,
			"/metrics": true,
		},
		TrackResponseBody: false,
		RequestIDHeader:   "X-Request-ID",
		SampleRate:        0.1,
	}
}

// Middleware returns Fiber middleware for automatic observability instrumentation
func (f *Framework) Middleware(config *MiddlewareConfig) fiber.Handler {
	if config == nil {
		config = DefaultMiddlewareConfig()
	}

	return func(c *fiber.Ctx) error {
		// Skip instrumentation for configured paths
		if config.SkipPaths[c.Path()] {
			return c.Next()
		}

		// Extract or generate correlation ID
		correlationID := c.Get(config.RequestIDHeader)
		if correlationID == "" {
			correlationID = generateTraceID()
		}

		// Create span for this request
		operation := fmt.Sprintf("%s %s", c.Method(), c.Path())
		spanCtx, span := f.StartSpan(c.UserContext(), operation)
		if span != nil {
			span.Attributes["correlation_id"] = correlationID
			span.Attributes["method"] = c.Method()
			span.Attributes["path"] = c.Path()
			span.Attributes["remote_ip"] = c.IP()
		}

		// Set correlation ID in response header
		c.Set(config.RequestIDHeader, correlationID)

		// Record request start
		startTime := time.Now()

		// Capture response status and error
		var responseErr error
		var statusCode int

		// Execute handler
		err := c.Next()

		// Capture response status
		statusCode = c.Response().StatusCode()

		// Mark span status based on response
		if err != nil {
			responseErr = err
			if span != nil {
				span.Status = "error"
				span.Attributes["error"] = err.Error()
			}
		} else if statusCode >= 400 {
			span.Status = "error"
			span.Attributes["status_code"] = statusCode
			if statusCode >= 500 {
				responseErr = fmt.Errorf("server error: %d", statusCode)
			}
		} else {
			if span != nil {
				span.Attributes["status_code"] = statusCode
			}
		}

		// Record request metrics
		duration := time.Since(startTime)
		f.RecordMetric(
			fmt.Sprintf("http_request_duration_seconds_%s", c.Method()),
			duration.Seconds(),
			map[string]string{
				"path":   c.Path(),
				"method": c.Method(),
				"status": fmt.Sprintf("%d", statusCode),
			},
		)

		// End span
		f.EndSpan(spanCtx, span, responseErr)

		// Log request
		f.logger.Info("http request",
			zap.String("correlation_id", correlationID),
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", statusCode),
			zap.Duration("duration", duration),
			zap.String("remote_ip", c.IP()),
			zap.Error(responseErr),
		)

		return err
	}
}

// ErrorHandler returns Fiber error handler for automatic observability
func (f *Framework) ErrorHandler(c *fiber.Ctx, err error) error {
	// Extract correlation ID
	correlationID := c.Get("X-Request-ID")
	if correlationID == "" {
		correlationID = generateTraceID()
	}

	// Determine status code
	code := fiber.StatusInternalServerError
	if fe, ok := err.(*fiber.Error); ok {
		code = fe.Code
	}

	// Log error with correlation ID
	f.logger.Error("request error",
		zap.String("correlation_id", correlationID),
		zap.String("method", c.Method()),
		zap.String("path", c.Path()),
		zap.Int("status_code", code),
		zap.Error(err),
	)

	// Record error metric
	f.RecordMetric(
		fmt.Sprintf("http_request_errors_%s", c.Method()),
		1.0,
		map[string]string{
			"path":   c.Path(),
			"method": c.Method(),
			"status": fmt.Sprintf("%d", code),
		},
	)

	// Return error response
	return c.Status(code).JSON(fiber.Map{
		"error": err.Error(),
		"correlation_id": correlationID,
	})
}

// RequestLogger returns middleware that logs all HTTP requests with correlation ID
func (f *Framework) RequestLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get or generate correlation ID
		correlationID := c.Get("X-Request-ID")
		if correlationID == "" {
			correlationID = generateTraceID()
			c.Set("X-Request-ID", correlationID)
		}

		// Log request
		f.logger.Info("incoming request",
			zap.String("correlation_id", correlationID),
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.String("remote_ip", c.IP()),
			zap.String("user_agent", c.Get("User-Agent")),
		)

		return c.Next()
	}
}

// PanicHandler returns middleware that catches and logs panics
func (f *Framework) PanicHandler() fiber.Handler {
	return func(c *fiber.Ctx) (err error) {
		defer func() {
			if r := recover(); r != nil {
				correlationID := c.Get("X-Request-ID")
				if correlationID == "" {
					correlationID = generateTraceID()
				}

				// Log panic
				f.logger.Error("panic recovered",
					zap.String("correlation_id", correlationID),
					zap.String("method", c.Method()),
					zap.String("path", c.Path()),
					zap.Any("panic", r),
				)

				// Record panic metric
				f.RecordMetric("panics_total", 1.0, map[string]string{
					"path": c.Path(),
				})

				// Respond with error
				err = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "internal server error",
					"correlation_id": correlationID,
				})
			}
		}()

		return c.Next()
	}
}

// ContextMiddleware injects framework into Fiber context for downstream handlers
func (f *Framework) ContextMiddleware(c *fiber.Ctx) error {
	// Store framework in context for access in handlers
	c.Locals("framework", f)

	// Store correlation ID for easy access
	correlationID := c.Get("X-Request-ID")
	if correlationID == "" {
		correlationID = generateTraceID()
		c.Set("X-Request-ID", correlationID)
	}
	c.Locals("correlation_id", correlationID)

	return c.Next()
}

// TimeoutHandler returns middleware that logs slow requests
func (f *Framework) TimeoutHandler(threshold time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start)

		if duration > threshold {
			correlationID := c.Get("X-Request-ID")
			f.logger.Warn("slow request detected",
				zap.String("correlation_id", correlationID),
				zap.String("method", c.Method()),
				zap.String("path", c.Path()),
				zap.Duration("duration", duration),
				zap.Duration("threshold", threshold),
			)

			f.RecordMetric("slow_requests_total", 1.0, map[string]string{
				"path": c.Path(),
			})
		}

		return err
	}
}
