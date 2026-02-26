package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config is an interface that configuration structs should implement.
type Config interface {
	Validate() error
}

// Load loads the configuration from files and environment variables.
func Load[T Config](cfg T, configName string, searchPaths []string, envPrefix string, defaults map[string]interface{}) (T, error) {
	v := viper.New()

	v.SetConfigName(configName)
	v.SetConfigType("yaml")

	for _, path := range searchPaths {
		v.AddConfigPath(path)
	}

	if envPrefix != "" {
		v.SetEnvPrefix(envPrefix)
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.AutomaticEnv()
	}

	for k, val := range defaults {
		v.SetDefault(k, val)
	}

	// Read configuration file if it exists
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return cfg, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found is OK, we'll use defaults and env vars
	}

	if err := v.Unmarshal(&cfg); err != nil {
		return cfg, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// MustLoad loads the configuration and panics on error.
func MustLoad[T Config](cfg T, configName string, searchPaths []string, envPrefix string, defaults map[string]interface{}) T {
	cfg, err := Load[T](cfg, configName, searchPaths, envPrefix, defaults)
	if err != nil {
		panic(err)
	}
	return cfg
}
