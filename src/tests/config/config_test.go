package config_test

import (
	"testing"

	"grade/src/platform/config"
)

func TestDefault(t *testing.T) {
	cfg := config.Default()
	if cfg.Port != "8080" {
		t.Errorf("Default().Port = %q, want %q", cfg.Port, "8080")
	}
}

func TestFromEnvOverride(t *testing.T) {
	t.Setenv("PORT", "9090")
	cfg := config.FromEnv()
	if cfg.Port != "9090" {
		t.Errorf("FromEnv().Port = %q, want %q", cfg.Port, "9090")
	}
}

func TestFromEnvFallback(t *testing.T) {
	t.Setenv("PORT", "")
	cfg := config.FromEnv()
	if cfg.Port != "8080" {
		t.Errorf("FromEnv().Port = %q, want %q", cfg.Port, "8080")
	}
}
