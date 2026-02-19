package logger

import (
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config represents logger configuration
type Config struct {
	Level       string   `json:"level"`
	Format      string   `json:"format"` // json, console
	Development bool     `json:"development"`
	OutputPaths []string `json:"output_paths"`
}

// Logger wraps zap.Logger with additional functionality
type Logger struct {
	*zap.Logger
	config Config
}

// New creates a new logger instance
func New(cfg Config) *zap.Logger {
	var zapCfg zap.Config
	
	if cfg.Development {
		zapCfg = zap.NewDevelopmentConfig()
	} else {
		zapCfg = zap.NewProductionConfig()
	}
	
	// Set log level
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}
	zapCfg.Level = zap.NewAtomicLevelAt(level)
	
	// Set encoding format
	if cfg.Format == "console" {
		zapCfg.Encoding = "console"
		zapCfg.EncoderConfig = zapcore.EncoderConfig{
			TimeKey:        "timestamp",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "message",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalColorLevelEncoder,
			EncodeTime:     customTimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}
	} else {
		zapCfg.Encoding = "json"
	}
	
	// Set output paths
	if len(cfg.OutputPaths) > 0 {
		zapCfg.OutputPaths = cfg.OutputPaths
	}
	
	logger, err := zapCfg.Build(
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to create logger: %v", err))
	}
	
	return logger
}

// customTimeEncoder formats time with a clean, readable format
func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05"))
}

// NewWithFields creates a logger with predefined fields
func NewWithFields(cfg Config, fields map[string]interface{}) *zap.Logger {
	logger := New(cfg)
	
	var zapFields []zap.Field
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	
	return logger.With(zapFields...)
}

// LoggerForSubsystem creates a logger specifically for a subsystem
func LoggerForSubsystem(cfg Config, subsystem string) *zap.Logger {
	return NewWithFields(cfg, map[string]interface{}{
		"subsystem": subsystem,
		"component": "sparki",
	})
}

// TUILogger creates a logger optimized for TUI applications
func TUILogger(cfg Config) *zap.Logger {
	// For TUI apps, we typically want to log to a file to avoid
	// interfering with the terminal UI
	tuiConfig := cfg
	tuiConfig.OutputPaths = []string{getLogFilePath()}
	tuiConfig.Format = "json" // JSON is better for file logging
	
	return NewWithFields(tuiConfig, map[string]interface{}{
		"component": "tui",
	})
}

// ServerLogger creates a logger optimized for server applications
func ServerLogger(cfg Config) *zap.Logger {
	return NewWithFields(cfg, map[string]interface{}{
		"component": "server",
	})
}

// StructuredFields helps create consistent structured log fields
type StructuredFields struct {
	fields []zap.Field
}

// NewStructuredFields creates a new structured fields builder
func NewStructuredFields() *StructuredFields {
	return &StructuredFields{
		fields: make([]zap.Field, 0),
	}
}

// String adds a string field
func (sf *StructuredFields) String(key, value string) *StructuredFields {
	sf.fields = append(sf.fields, zap.String(key, value))
	return sf
}

// Int adds an integer field
func (sf *StructuredFields) Int(key string, value int) *StructuredFields {
	sf.fields = append(sf.fields, zap.Int(key, value))
	return sf
}

// Duration adds a duration field
func (sf *StructuredFields) Duration(key string, value time.Duration) *StructuredFields {
	sf.fields = append(sf.fields, zap.Duration(key, value))
	return sf
}

// Error adds an error field
func (sf *StructuredFields) Error(err error) *StructuredFields {
	sf.fields = append(sf.fields, zap.Error(err))
	return sf
}

// Any adds any type field
func (sf *StructuredFields) Any(key string, value interface{}) *StructuredFields {
	sf.fields = append(sf.fields, zap.Any(key, value))
	return sf
}

// Build returns the constructed fields
func (sf *StructuredFields) Build() []zap.Field {
	return sf.fields
}

// Helper function to get log file path
func getLogFilePath() string {
	// In a real implementation, this would use proper path resolution
	// based on the application's data directory
	return "/tmp/sparki.log"
}

// Predefined field constructors for common use cases
func SubsystemField(subsystem string) zap.Field {
	return zap.String("subsystem", subsystem)
}

func OperationField(operation string) zap.Field {
	return zap.String("operation", operation)
}

func ProjectField(project string) zap.Field {
	return zap.String("project", project)
}

func UserField(user string) zap.Field {
	return zap.String("user", user)
}

func RequestIDField(requestID string) zap.Field {
	return zap.String("request_id", requestID)
}

// LogLevel constants for consistency
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
	LevelFatal = "fatal"
)

// IsValidLogLevel checks if a log level string is valid
func IsValidLogLevel(level string) bool {
	validLevels := []string{LevelDebug, LevelInfo, LevelWarn, LevelError, LevelFatal}
	for _, valid := range validLevels {
		if strings.EqualFold(level, valid) {
			return true
		}
	}
	return false
}