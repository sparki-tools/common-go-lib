# Common Go Library

Shared Go packages for Sparki services.

## Overview

This library provides common utilities used across Sparki Go services:

- **logger** - Structured logging with context
- **config** - Configuration management
- **middleware** - HTTP middleware (auth, tracing, metrics)
- **types** - Shared type definitions
- **testing** - Test utilities and mocks

## Installation

```go
go get github.com/sparki-tools/common-go-lib
```

## Usage

```go
import (
    "github.com/sparki-tools/common-go-lib/logger"
    "github.com/sparki-tools/common-go-lib/config"
)

// Initialize logger
log := logger.New(logger.WithJSON())
log.Info("service started", "port", 8080)

// Load config
cfg, err := config.Load()
```

## Directory Structure

```
common-go-lib/
├── logger/         # Structured logging
├── config/         # Configuration management
├── middleware/     # HTTP middleware
├── types/          # Shared types
├── testing/        # Test utilities
├── go.mod
└── README.md
```

## Versioning

This library follows semantic versioning. See [CHANGELOG.md](CHANGELOG.md) for release notes.

## Related Repositories

- [engine](https://github.com/sparki-tools/engine)
- [api-contracts](https://github.com/sparki-tools/api-contracts)

## License

MIT
