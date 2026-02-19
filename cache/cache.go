package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Config holds Redis cache configuration
type Config struct {
	Addr     string
	Password string
	DB       int
	// Connection pool settings
	PoolSize     int
	MinIdleConns int
	MaxRetries   int
	// Timeouts
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	// Health check
	PoolTimeout time.Duration
}

// Cache wraps Redis client with domain-specific operations
type Cache struct {
	client *redis.Client
	logger *zap.Logger
	config *Config
}

// New creates a new cache instance
func New(cfg *Config, logger *zap.Logger) (*Cache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		MaxRetries:   cfg.MaxRetries,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolTimeout:  cfg.PoolTimeout,
	})

	cache := &Cache{
		client: client,
		logger: logger,
		config: cfg,
	}

	// Verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := cache.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Redis cache connected",
		zap.String("addr", cfg.Addr),
		zap.Int("db", cfg.DB),
		zap.Int("pool_size", cfg.PoolSize),
	)

	return cache, nil
}

// Client returns the underlying Redis client
func (c *Cache) Client() *redis.Client {
	return c.client
}

// Close closes the Redis connection
func (c *Cache) Close() error {
	c.logger.Info("closing Redis connection")
	return c.client.Close()
}

// Ping verifies Redis connectivity
func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Health returns detailed health information
func (c *Cache) Health(ctx context.Context) (*HealthStatus, error) {
	health := &HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Check connectivity
	start := time.Now()
	if err := c.client.Ping(ctx).Err(); err != nil {
		health.Status = "unhealthy"
		health.Error = err.Error()
		return health, err
	}
	health.Details["ping_duration_ms"] = time.Since(start).Milliseconds()

	// Get pool stats
	stats := c.client.PoolStats()
	health.Details["hits"] = stats.Hits
	health.Details["misses"] = stats.Misses
	health.Details["timeouts"] = stats.Timeouts
	health.Details["total_conns"] = stats.TotalConns
	health.Details["idle_conns"] = stats.IdleConns
	health.Details["stale_conns"] = stats.StaleConns

	// Get server info
	info := c.client.Info(ctx, "server")
	if info.Err() != nil {
		health.Status = "degraded"
		health.Details["info_error"] = info.Err().Error()
	}

	// Check memory usage
	memInfo := c.client.Info(ctx, "memory")
	if memInfo.Err() == nil {
		health.Details["memory_stats"] = "available"
	}

	return health, nil
}

// Stats returns current pool statistics
func (c *Cache) Stats() *PoolStats {
	stats := c.client.PoolStats()
	return &PoolStats{
		Hits:       stats.Hits,
		Misses:     stats.Misses,
		Timeouts:   stats.Timeouts,
		TotalConns: stats.TotalConns,
		IdleConns:  stats.IdleConns,
		StaleConns: stats.StaleConns,
	}
}

// HealthStatus represents cache health
type HealthStatus struct {
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details"`
	Error     string                 `json:"error,omitempty"`
}

// PoolStats represents connection pool statistics
type PoolStats struct {
	Hits       uint32 `json:"hits"`
	Misses     uint32 `json:"misses"`
	Timeouts   uint32 `json:"timeouts"`
	TotalConns uint32 `json:"total_conns"`
	IdleConns  uint32 `json:"idle_conns"`
	StaleConns uint32 `json:"stale_conns"`
}

// DefaultConfig returns sensible default configuration
func DefaultConfig() *Config {
	return &Config{
		Addr:         "localhost:6379",
		Password:     "",
		DB:           0,
		PoolSize:     10,
		MinIdleConns: 5,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
	}
}
