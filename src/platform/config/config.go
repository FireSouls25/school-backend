// Package config loads runtime configuration from the environment.
package config

import "os"

// Config holds the runtime configuration for the backend.
type Config struct {
	// Port is the TCP port the HTTP server listens on.
	Port string
}

// Default returns a Config with production-sane defaults.
func Default() Config {
	return Config{Port: "8080"}
}

// FromEnv builds a Config from environment variables, falling back to Default.
func FromEnv() Config {
	cfg := Default()
	if v := os.Getenv("PORT"); v != "" {
		cfg.Port = v
	}
	return cfg
}
