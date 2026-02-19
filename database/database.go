package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Config holds database configuration
type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DB wraps pgxpool with additional functionality
type DB struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
	config *Config
}

// New creates a new database connection pool
func New(ctx context.Context, cfg *Config, logger *zap.Logger) (*DB, error) {
	// Build connection string
	connString := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode,
	)

	// Parse config
	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Set pool configuration
	poolConfig.MaxConns = int32(cfg.MaxOpenConns)
	poolConfig.MinConns = int32(cfg.MaxIdleConns)
	poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime
	poolConfig.MaxConnIdleTime = cfg.ConnMaxIdleTime

	// Set up connection hooks for observability
	poolConfig.BeforeAcquire = func(ctx context.Context, conn *pgx.Conn) bool {
		logger.Debug("acquiring database connection")
		return true
	}

	poolConfig.AfterRelease = func(conn *pgx.Conn) bool {
		logger.Debug("releasing database connection")
		return true
	}

	// Create pool
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	db := &DB{
		pool:   pool,
		logger: logger,
		config: cfg,
	}

	// Verify connectivity
	if err := db.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("database connection pool created",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Database),
		zap.Int("max_conns", cfg.MaxOpenConns),
	)

	return db, nil
}

// Pool returns the underlying pgxpool.Pool
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// Close closes the database connection pool
func (db *DB) Close() {
	db.logger.Info("closing database connection pool")
	db.pool.Close()
}

// Ping verifies database connectivity
func (db *DB) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.pool.Ping(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// Health returns detailed health information
func (db *DB) Health(ctx context.Context) (*HealthStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	health := &HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
	}

	// Check connectivity
	start := time.Now()
	if err := db.pool.Ping(ctx); err != nil {
		health.Status = "unhealthy"
		health.Error = err.Error()
		return health, err
	}
	health.Details["ping_duration_ms"] = time.Since(start).Milliseconds()

	// Get pool stats
	stats := db.pool.Stat()
	health.Details["total_conns"] = stats.TotalConns()
	health.Details["acquired_conns"] = stats.AcquiredConns()
	health.Details["idle_conns"] = stats.IdleConns()
	health.Details["max_conns"] = stats.MaxConns()

	// Query database version
	var version string
	err := db.pool.QueryRow(ctx, "SELECT version()").Scan(&version)
	if err != nil {
		health.Status = "degraded"
		health.Details["version_error"] = err.Error()
	} else {
		health.Details["version"] = version
	}

	// Check for long-running queries (> 30 seconds)
	var longRunningCount int
	err = db.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_stat_activity
		WHERE state = 'active'
		AND query_start < NOW() - INTERVAL '30 seconds'
		AND query NOT LIKE '%pg_stat_activity%'
	`).Scan(&longRunningCount)
	if err == nil && longRunningCount > 0 {
		health.Status = "degraded"
		health.Details["long_running_queries"] = longRunningCount
	}

	return health, nil
}

// Stats returns current pool statistics
func (db *DB) Stats() *PoolStats {
	stats := db.pool.Stat()
	return &PoolStats{
		TotalConns:    stats.TotalConns(),
		AcquiredConns: stats.AcquiredConns(),
		IdleConns:     stats.IdleConns(),
		MaxConns:      stats.MaxConns(),
	}
}

// WithTransaction executes a function within a transaction
func (db *DB) WithTransaction(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p) // Re-throw panic after rollback
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			db.logger.Error("failed to rollback transaction",
				zap.Error(rbErr),
				zap.Error(err),
			)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// HealthStatus represents database health
type HealthStatus struct {
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Details   map[string]interface{} `json:"details"`
	Error     string                 `json:"error,omitempty"`
}

// PoolStats represents connection pool statistics
type PoolStats struct {
	TotalConns    int32 `json:"total_conns"`
	AcquiredConns int32 `json:"acquired_conns"`
	IdleConns     int32 `json:"idle_conns"`
	MaxConns      int32 `json:"max_conns"`
}

// DefaultConfig returns sensible default configuration
func DefaultConfig() *Config {
	return &Config{
		Host:            "localhost",
		Port:            5432,
		User:            "sparki",
		Password:        "sparki",
		Database:        "sparki_development",
		SSLMode:         "disable",
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
	}
}
