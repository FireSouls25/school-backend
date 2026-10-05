// Package config loads runtime configuration from the environment.
//
// Two deployment shapes share the same code:
//
//   - development: in-memory or local Postgres, demo data allowed,
//     APP_ENV unset (defaults to development).
//   - production: APP_ENV=production enforces the safety rules in
//     Validate (real database, no demo data, no known dev passwords).
package config

import (
	"fmt"
	"os"
	"strings"
)

// AppEnv selects the deployment shape and the safety rules that apply.
type AppEnv string

const (
	// EnvDevelopment is the default: handy defaults, demo data allowed.
	EnvDevelopment AppEnv = "development"
	// EnvProduction enforces Validate: real database, no demo seeds.
	EnvProduction AppEnv = "production"
)

// IsProduction reports whether e is the production environment.
func (e AppEnv) IsProduction() bool { return e == EnvProduction }

// BootstrapAdmin is the first account created at boot, so an empty
// installation has someone who can grant roles and create users. Empty
// Username+Password means "no bootstrap" (allowed once real accounts
// exist). Password may come from BOOTSTRAP_ADMIN_PASSWORD or from
// BOOTSTRAP_ADMIN_PASSWORD_FILE (docker/k8s secrets).
type BootstrapAdmin struct {
	Username string
	Email    string
	Password string
}

// Enabled reports whether a bootstrap account was requested.
func (b BootstrapAdmin) Enabled() bool {
	return b.Username != "" && b.Password != ""
}

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
	// Env is the deployment shape (development by default).
	Env AppEnv
	// SeedDemo creates the demo school at boot when Env is development.
	// Defaults to true in development, forced false in production.
	SeedDemo bool
	// Bootstrap is the optional first admin account.
	Bootstrap BootstrapAdmin
}

// devPasswords are the passwords shipped in the demo data and docs. They
// must never reach production; Validate rejects them.
var devPasswords = []string{"admin123*", "docente123*", "temporal123*", "admin123", "docente123"}

// Default returns a Config with production-sane defaults for local work.
func Default() Config {
	return Config{Port: "8080", Env: EnvDevelopment, SeedDemo: true}
}

// FromEnv builds a Config from environment variables, falling back to
// Default. A malformed APP_ENV is an error: guessing would silently
// downgrade a production deployment to development rules.
func FromEnv() (Config, error) {
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
	env, err := parseEnv(os.Getenv("APP_ENV"))
	if err != nil {
		return Config{}, err
	}
	cfg.Env = env

	// SEED_DEMO defaults to on in development and always off in
	// production, so the flag can never weaken prod by omission.
	if v := strings.TrimSpace(os.Getenv("SEED_DEMO")); v != "" {
		on, err := parseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: SEED_DEMO: %w", err)
		}
		cfg.SeedDemo = on
	}
	if cfg.Env.IsProduction() {
		cfg.SeedDemo = false
	}

	cfg.Bootstrap = BootstrapAdmin{
		Username: strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_USERNAME")),
		Email:    strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL")),
	}
	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if file := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_PASSWORD_FILE")); file != "" {
		secret, err := readSecretFile(file)
		if err != nil {
			return Config{}, err
		}
		password = secret
	}
	cfg.Bootstrap.Password = strings.TrimRight(password, "\r\n")

	// A half-configured bootstrap silently disables the first admin:
	// treat it as a configuration error instead of starting a system
	// nobody can administer.
	if cfg.Bootstrap.Username != "" && cfg.Bootstrap.Password == "" {
		return Config{}, fmt.Errorf(
			"config: BOOTSTRAP_ADMIN_USERNAME set without BOOTSTRAP_ADMIN_PASSWORD")
	}
	if cfg.Bootstrap.Username == "" && cfg.Bootstrap.Password != "" {
		return Config{}, fmt.Errorf(
			"config: BOOTSTRAP_ADMIN_PASSWORD set without BOOTSTRAP_ADMIN_USERNAME")
	}
	return cfg, nil
}

// Validate enforces the rules that must never be violated in production.
func (c Config) Validate() error {
	if !c.Env.IsProduction() {
		return nil
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("config: DATABASE_URL is required when APP_ENV=production")
	}
	if c.SeedDemo {
		return fmt.Errorf("config: SEED_DEMO cannot be enabled when APP_ENV=production")
	}
	if c.Bootstrap.Enabled() && isDevPassword(c.Bootstrap.Password) {
		return fmt.Errorf(
			"config: BOOTSTRAP_ADMIN_PASSWORD is a known development password; " +
				"pick a unique one before deploying")
	}
	return nil
}

// UsesMemoryStores reports whether the deployment would fall back to the
// in-memory stores (development only; data is lost on every restart).
func (c Config) UsesMemoryStores() bool { return c.DatabaseURL == "" }

func isDevPassword(password string) bool {
	for _, p := range devPasswords {
		if password == p {
			return true
		}
	}
	return false
}

func parseEnv(raw string) (AppEnv, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "dev", "development":
		return EnvDevelopment, nil
	case "prod", "production":
		return EnvProduction, nil
	default:
		return "", fmt.Errorf("config: APP_ENV %q is not development|production", raw)
	}
}

func parseBool(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "t", "true", "yes", "on":
		return true, nil
	case "0", "f", "false", "no", "off", "":
		return false, nil
	default:
		return false, fmt.Errorf("config: %q is not a boolean", raw)
	}
}

// readSecretFile loads a secret from disk (docker/k8s secrets). Only the
// trailing newline written by most secret tooling is trimmed; spaces are
// part of the secret.
func readSecretFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("config: read secret file: %w", err)
	}
	return string(b), nil
}
