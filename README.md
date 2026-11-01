# Common Go Library

Shared Go packages for Sparki services.

## Overview

This library provides common utilities used across Sparki Go services:

- **logger** - Structured logging with Zap
- **observability** - Unified metrics, tracing, and profiling framework
- **integration** - Service registry and client factories
- **storm** - Storm observability platform client
- **cache** - Redis cache wrapper with connection pooling
- **database** - PostgreSQL connection pool with pgx
- **testing** - Test utilities, assertions, and concurrency helpers
- **config** - Configuration management
- **middleware** - HTTP middleware (auth, tracing, metrics)
- **types** - Shared type definitions
- **githubapp** - GitHub App JWT signing and cached installation tokens (stdlib only)

## Installation

```go
go get github.com/sparki-tools/common-go-lib
```

## Usage

### Logger

```go
import "github.com/sparki-tools/common-go-lib/logger"

cfg := logger.Config{
    Level:       "info",
    Format:      "json",
    Development: false,
}
log := logger.New(cfg)
log.Info("service started", zap.Int("port", 8080))
```

### Observability Framework

```go
import "github.com/sparki-tools/common-go-lib/observability"

config := &observability.ObservabilityConfig{
    MetricsEnabled: true,
    TracingEnabled: true,
    ServiceName:    "my-service",
}
framework, err := observability.NewFramework(config)

// Create spans
ctx, span := framework.StartSpan(ctx, "operation")
defer framework.EndSpan(ctx, span, nil)
```

### Cache

```go
import "github.com/sparki-tools/common-go-lib/cache"

cfg := cache.DefaultConfig()
cfg.Addr = "localhost:6379"
cache, err := cache.New(cfg, logger)

// Health check
health, err := cache.Health(ctx)
```

### GitHub App

```go
import "github.com/sparki-tools/common-go-lib/githubapp"

app, err := githubapp.New(githubapp.Config{AppID: appID, PrivateKeyPEM: keyPEM})

// Resolve a repo to its installation and numeric repo ID.
instID, repoID, err := app.Installation(ctx, "owner", "repo")

// Repo-scoped, read-only token; cached until 5 minutes before expiry.
tok, err := app.InstallationToken(ctx, instID, githubapp.TokenOptions{
    RepositoryIDs: []int64{repoID},
    Permissions:   map[string]string{"contents": "read"},
})

// Branch on errors.Is(err, githubapp.ErrNotInstalled | ErrUnauthorized | ErrRateLimited).
```

### Database

```go
import "github.com/sparki-tools/common-go-lib/database"

cfg := database.DefaultConfig()
db, err := database.New(ctx, cfg, logger)

// Transaction helper
err = db.WithTransaction(ctx, func(tx pgx.Tx) error {
    // ... execute queries
    return nil
})
```

### Service Registry

```go
import "github.com/sparki-tools/common-go-lib/integration"

registry := integration.NewServiceRegistry()
registry.Register(&integration.ServiceEndpoint{
    Type:     integration.ServiceTypeAPI,
    Host:     "api.sparki.tools",
    Port:     8080,
    Protocol: "https",
})
```

### Storm Client

```go
import "github.com/sparki-tools/common-go-lib/storm"

client, err := storm.NewClient(&storm.ClientConfig{
    Endpoint: "http://storm:8080",
    Timeout:  30 * time.Second,
})

// Publish metrics
client.PublishMetrics(ctx, &storm.MetricsPayload{
    ServiceName: "my-service",
    Metrics:     map[string]interface{}{"requests": 100},
})
```

### Testing Utilities

```go
import sparki_testing "github.com/sparki-tools/common-go-lib/testing"

// Assertions
sparki_testing.AssertEqual(t, expected, actual, "values should match")
sparki_testing.AssertNoError(t, err, "should not error")

// Concurrency testing
ct := sparki_testing.NewConcurrencyTest(10, func(ctx context.Context, id int) error {
    // ... concurrent operation
    return nil
})
err := ct.Execute(ctx)
```

## Directory Structure

```
common-go-lib/
├── cache/           # Redis cache wrapper
├── config/          # Configuration management
├── database/        # PostgreSQL connection pool
├── integration/     # Service registry and client factories
├── logger/          # Structured logging with Zap
├── middleware/      # HTTP middleware
├── observability/   # Metrics, tracing, profiling
├── storm/           # Storm platform client
├── testing/         # Test utilities and assertions
├── types/           # Shared types
├── go.mod
└── README.md
```

## Package Dependencies

| Package | External Dependencies |
|---------|----------------------|
| logger | go.uber.org/zap |
| observability | prometheus/client_golang, zap |
| cache | redis/go-redis, zap |
| database | jackc/pgx/v5, zap |
| storm | net/http, zap |
| integration | (stdlib only) |
| testing | prometheus/client_golang, zap |

## Versioning

This library follows semantic versioning. See [CHANGELOG.md](CHANGELOG.md) for release notes.

## Related Repositories

- [api-engine](https://github.com/sparki-tools/api-engine) - Main backend service
- [api-contracts](https://github.com/sparki-tools/api-contracts) - API specifications
- [observability-storm](https://github.com/sparki-tools/observability-storm) - Observability platform

## License

MIT
