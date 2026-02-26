package middleware

import (
	"fmt"
	"runtime/debug"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/sparki-tools/common-go-lib/logger"
	"go.uber.org/zap"
)

// Recovery recovers from panics and logs the stack trace
func Recovery(log *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				err, ok := r.(error)
				if !ok {
					err = fmt.Errorf("%v", r)
				}
				stack := debug.Stack()

				if log != nil {
					log.Error("Panic recovered",
						zap.Error(err),
						zap.String("stack", string(stack)),
						zap.String("path", c.Path()),
						zap.String("method", c.Method()),
					)
				} else {
					fmt.Printf("Panic recovered: %v\n%s\n", err, stack)
				}

				c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error":   true,
					"message": "Internal Server Error",
					"code":    500,
				})
			}
		}()
		return c.Next()
	}
}

// RequestID adds a unique ID to each request
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		rid := c.Get(fiber.HeaderXRequestID)
		if rid == "" {
			rid = uuid.New().String()
		}
		c.Set(fiber.HeaderXRequestID, rid)
		c.Locals("requestid", rid)
		return c.Next()
	}
}

// Timeout adds a timeout to the request context
func Timeout(timeout time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Note: Fiber's context doesn't support built-in cancellation via context.Context
		// for standard handlers unless using a specific timeout middleware.
		// This is a placeholder for architectural consistency.
		return c.Next()
	}
}

// Chain allows combining multiple handlers into one
func Chain(handlers ...fiber.Handler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		for _, h := range handlers {
			if err := h(c); err != nil {
				return err
			}
		}
		return nil
	}
}

// RequestTimer adds request timing to context
func RequestTimer() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("requestTime", time.Now())
		return c.Next()
	}
}

// SparkiHeaders adds Sparki-specific headers
func SparkiHeaders(version string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Powered-By", "Sparki")
		if version != "" {
			c.Set("X-Sparki-Version", version)
		}
		return c.Next()
	}
}

// APIKeyAuth provides API key authentication
func APIKeyAuth(validKeys []string) fiber.Handler {
	keyMap := make(map[string]bool)
	for _, key := range validKeys {
		keyMap[key] = true
	}

	return func(c *fiber.Ctx) error {
		apiKey := c.Get("X-API-Key")
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}

		if apiKey == "" || !keyMap[apiKey] {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   true,
				"message": "Invalid or missing API key",
				"code":    401,
			})
		}

		c.Locals("apiKey", apiKey)
		return c.Next()
	}
}

// ProjectContext adds project context to requests
func ProjectContext() fiber.Handler {
	return func(c *fiber.Ctx) error {
		projectID := c.Get("X-Project-ID")
		if projectID == "" {
			projectID = c.Query("project_id")
		}

		if projectID != "" {
			c.Locals("projectID", projectID)
		}

		return c.Next()
	}
}

// RequestLogger provides structured request logging
func RequestLogger(log *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Process request
		err := c.Next()

		if log == nil {
			return err
		}

		// Log request details
		fields := logger.NewStructuredFields().
			String("method", c.Method()).
			String("path", c.Path()).
			String("ip", c.IP()).
			String("user_agent", c.Get("User-Agent")).
			Int("status", c.Response().StatusCode()).
			Duration("latency", time.Since(start))

		if reqID := c.Locals("requestid"); reqID != nil {
			fields.String("request_id", reqID.(string))
		}

		if projectID := c.Locals("projectID"); projectID != nil {
			fields.String("project_id", projectID.(string))
		}

		if err != nil {
			fields.Error(err)
			log.Error("HTTP request failed", fields.Build()...)
		} else {
			log.Info("HTTP request", fields.Build()...)
		}

		return err
	}
}

// RateLimit provides basic rate limiting (In-memory)
func RateLimit(maxRequests int, window time.Duration) fiber.Handler {
	// Simple in-memory rate limiter
	// Note: For production use, a distributed store like Redis is recommended.
	type client struct {
		requests []time.Time
	}
	clients := make(map[string]*client)

	return func(c *fiber.Ctx) error {
		clientIP := c.IP()
		now := time.Now()

		if _, exists := clients[clientIP]; !exists {
			clients[clientIP] = &client{}
		}

		// Clean old requests
		var validRequests []time.Time
		for _, t := range clients[clientIP].requests {
			if now.Sub(t) <= window {
				validRequests = append(validRequests, t)
			}
		}

		if len(validRequests) >= maxRequests {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       true,
				"message":     "Rate limit exceeded",
				"code":        429,
				"retry_after": window.Seconds(),
			})
		}

		validRequests = append(validRequests, now)
		clients[clientIP].requests = validRequests

		return c.Next()
	}
}

// CORSSparki provides CORS with default Sparki configuration
func CORSSparki(allowedOrigins []string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		origin := c.Get("Origin")

		allowed := false
		for _, o := range allowedOrigins {
			if origin == o || o == "*" {
				allowed = true
				break
			}
		}

		if allowed {
			c.Set("Access-Control-Allow-Origin", origin)
		}

		c.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Set("Access-Control-Allow-Headers", "Origin,Content-Type,Accept,Authorization,X-Request-ID,X-API-Key,X-Project-ID")
		c.Set("Access-Control-Allow-Credentials", "false")
		c.Set("Access-Control-Max-Age", "3600")

		if c.Method() == "OPTIONS" {
			return c.SendStatus(fiber.StatusNoContent)
		}

		return c.Next()
	}
}

// HealthCheck provides health check responder
func HealthCheck(healthPath string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Path() == healthPath {
			return c.JSON(fiber.Map{
				"status":    "healthy",
				"timestamp": time.Now().UTC(),
			})
		}
		return c.Next()
	}
}

// ErrorHandler provides consistent error responses
func ErrorHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()
		if err == nil {
			return nil
		}

		code := fiber.StatusInternalServerError
		message := "Internal Server Error"

		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
			message = e.Message
		}

		errorResponse := fiber.Map{
			"error":     true,
			"message":   message,
			"code":      code,
			"timestamp": time.Now().UTC(),
		}

		if reqID := c.Locals("requestid"); reqID != nil {
			errorResponse["request_id"] = reqID
		}

		if dev := c.Locals("development"); dev == true {
			errorResponse["stack"] = err.Error()
		}

		return c.Status(code).JSON(errorResponse)
	}
}

// DevelopmentMode adds development-specific features
func DevelopmentMode(isDev bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if isDev {
			c.Locals("development", true)
			c.Set("X-Development-Mode", "true")
			c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Set("Pragma", "no-cache")
			c.Set("Expires", "0")
		}
		return c.Next()
	}
}
