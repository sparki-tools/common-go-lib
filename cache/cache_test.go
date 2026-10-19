package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNew(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "valid connection",
			config:  testConfig(),
			wantErr: false,
		},
		{
			name: "invalid address",
			config: &Config{
				Addr:         "invalid:9999",
				Password:     "",
				DB:           0,
				PoolSize:     10,
				MinIdleConns: 5,
				MaxRetries:   3,
				DialTimeout:  1 * time.Second,
				ReadTimeout:  1 * time.Second,
				WriteTimeout: 1 * time.Second,
				PoolTimeout:  1 * time.Second,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := New(tt.config, logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && cache != nil {
				defer cache.Close()

				// Verify connection works
				if err := cache.Ping(context.Background()); err != nil {
					t.Errorf("Ping() failed: %v", err)
				}
			}
		})
	}
}

func TestCache_Health(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := testConfig()

	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("Cannot connect to Redis: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	health, err := cache.Health(ctx)
	if err != nil {
		t.Errorf("Health() error = %v", err)
		return
	}

	if health.Status != "healthy" && health.Status != "degraded" {
		t.Errorf("Health() status = %v, want healthy or degraded", health.Status)
	}

	// Check ping duration
	if _, ok := health.Details["ping_duration_ms"]; !ok {
		t.Error("Health() missing ping_duration_ms")
	}

	// Check pool stats
	if _, ok := health.Details["hits"]; !ok {
		t.Error("Health() missing hits")
	}
}

func TestCache_Stats(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := testConfig()

	cache, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("Cannot connect to Redis: %v", err)
	}
	defer cache.Close()

	stats := cache.Stats()
	if stats == nil {
		t.Fatal("Stats() returned nil")
	}

	// Stats should have valid fields (may be zero initially)
	if stats.TotalConns < 0 {
		t.Errorf("Stats().TotalConns = %d, want >= 0", stats.TotalConns)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Addr == "" {
		t.Error("DefaultConfig().Addr is empty")
	}

	if cfg.PoolSize <= 0 {
		t.Errorf("DefaultConfig().PoolSize = %d, want > 0", cfg.PoolSize)
	}

	if cfg.DialTimeout <= 0 {
		t.Errorf("DefaultConfig().DialTimeout = %v, want > 0", cfg.DialTimeout)
	}
}

// testConfig points at the test Redis. TEST_REDIS_ADDR overrides the default,
// as in api-engine; the Mac lab runs Redis on 56379.
func testConfig() *Config {
	cfg := DefaultConfig()

	if v := os.Getenv("TEST_REDIS_ADDR"); v != "" {
		cfg.Addr = v
	}

	return cfg
}
