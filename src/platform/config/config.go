// Package config loads runtime configuration from the environment.
package config

import (
	"os"
	"strings"
)

// Config holds the runtime configuration for the backend.
type Config struct {
	// Port is the TCP port the HTTP server listens on.
	Port string
	// DatabaseURL is the PostgreSQL connection string. When empty the
	// in-memory stores are used (development only).
	DatabaseURL string
	// AllowedOrigins lists browser origins accepted by CORS,
	// comma-separated. Empty means same-origin only.
	AllowedOrigins []string
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
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("ALLOWED_ORIGINS"); v != "" {
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
			}
		}
	}
	return cfg
}
