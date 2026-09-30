// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds all settings the API needs at startup.
type Config struct {
	Addr            string        // listen address, e.g. ":8080"
	ShutdownTimeout time.Duration // max time to drain in-flight requests on SIGTERM
}

// Load reads configuration from the environment, applying defaults.
// It returns an error instead of exiting so main decides how to fail.
func Load() (Config, error) {
	cfg := Config{
		Addr:            getenv("ADDR", ":8080"),
		ShutdownTimeout: 20 * time.Second,
	}

	if v := os.Getenv("SHUTDOWN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v) // accepts "15s", "1m", ...
		if err != nil {
			return Config{}, fmt.Errorf("parse SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.ShutdownTimeout = d
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
