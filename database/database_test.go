package database

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

func TestNew(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	ctx := context.Background()

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "valid connection",
			config:  testConfig(t),
			wantErr: false,
		},
		{
			name: "invalid host",
			config: &Config{
				Host:            "invalid-host",
				Port:            5432,
				User:            "sparki_test",
				Password:        "sparki_test",
				Database:        "postgres",
				SSLMode:         "disable",
				MaxOpenConns:    5,
				MaxIdleConns:    2,
				ConnMaxLifetime: 5 * time.Minute,
				ConnMaxIdleTime: 1 * time.Minute,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := New(ctx, tt.config, logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && db != nil {
				defer db.Close()

				// Verify connection works
				if err := db.Ping(ctx); err != nil {
					t.Errorf("Ping() failed: %v", err)
				}
			}
		})
	}
}

func TestDB_Health(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	ctx := context.Background()

	cfg := testConfig(t)

	db, err := New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Cannot connect to test database: %v", err)
	}
	defer db.Close()

	health, err := db.Health(ctx)
	if err != nil {
		t.Errorf("Health() error = %v", err)
		return
	}

	if health.Status != "healthy" && health.Status != "degraded" {
		t.Errorf("Health() status = %v, want healthy or degraded", health.Status)
	}

	// Check that ping duration is recorded
	if _, ok := health.Details["ping_duration_ms"]; !ok {
		t.Error("Health() missing ping_duration_ms")
	}

	// Check that pool stats are recorded
	if _, ok := health.Details["total_conns"]; !ok {
		t.Error("Health() missing total_conns")
	}
}

func TestDB_Stats(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	ctx := context.Background()

	cfg := testConfig(t)
	db, err := New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Cannot connect to test database: %v", err)
	}
	defer db.Close()

	stats := db.Stats()
	if stats == nil {
		t.Fatal("Stats() returned nil")
	}

	if stats.MaxConns != int32(cfg.MaxOpenConns) {
		t.Errorf("Stats().MaxConns = %d, want %d", stats.MaxConns, cfg.MaxOpenConns)
	}
}

func TestDB_WithTransaction(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	ctx := context.Background()

	cfg := testConfig(t)
	db, err := New(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Cannot connect to test database: %v", err)
	}
	defer db.Close()

	t.Run("successful transaction", func(t *testing.T) {
		err := db.WithTransaction(ctx, func(tx pgx.Tx) error {
			// Transaction should succeed
			return nil
		})

		if err != nil {
			t.Errorf("WithTransaction() error = %v, want nil", err)
		}
	})

	t.Run("transaction rollback on error", func(t *testing.T) {
		testErr := fmt.Errorf("test error")
		err := db.WithTransaction(ctx, func(tx pgx.Tx) error {
			return testErr
		})

		if err != testErr {
			t.Errorf("WithTransaction() error = %v, want %v", err, testErr)
		}
	})
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Host != "localhost" {
		t.Errorf("DefaultConfig().Host = %v, want localhost", cfg.Host)
	}

	if cfg.Port != 5432 {
		t.Errorf("DefaultConfig().Port = %v, want 5432", cfg.Port)
	}

	if cfg.MaxOpenConns <= 0 {
		t.Errorf("DefaultConfig().MaxOpenConns = %v, want > 0", cfg.MaxOpenConns)
	}
}

// Helper
func err(msg string) error {
	return &testError{msg: msg}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// testConfig points at the test Postgres. TEST_DB_* override the defaults, as
// in api-engine; the Mac lab runs Postgres on 55432.
// testConfig returns the test database config, or skips the test when
// TEST_DB_HOST is unset: these tests need a live Postgres, which the Mac lab
// provides (lab-ci.sh exports TEST_DB_*) and a bare `go test` does not.
func testConfig(t *testing.T) *Config {
	t.Helper()

	if os.Getenv("TEST_DB_HOST") == "" {
		t.Skip("TEST_DB_HOST unset: no test database")
	}

	port := 5432

	if v := os.Getenv("TEST_DB_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("TEST_DB_PORT=%q: %v", v, err)
		}

		port = p
	}

	return &Config{
		Host:            envOr("TEST_DB_HOST", "localhost"),
		Port:            port,
		User:            envOr("TEST_DB_USER", "sparki_test"),
		Password:        envOr("TEST_DB_PASSWORD", "sparki_test"),
		Database:        "postgres",
		SSLMode:         "disable",
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}
