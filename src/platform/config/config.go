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
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// ParseTrustedProxies validates comma-separated IPs/CIDRs; empty trusts none.
func ParseTrustedProxies(raw string) (bool, error) {
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "/") {
			if _, _, err := net.ParseCIDR(part); err != nil {
				return false, err
			}
			continue
		}
		if net.ParseIP(part) == nil {
			return false, &net.ParseError{Type: "IP address", Text: part}
		}
	}
	return true, nil
}

// DefaultAuthTTL is the legacy login token lifetime, kept as the access
// fallback when AUTH_ACCESS_TTL is unset.
const DefaultAuthTTL = 12 * time.Hour

// DefaultAuthAccessTTL is the short API bearer lifetime: a stolen access
// token dies on its own in minutes.
const DefaultAuthAccessTTL = 15 * time.Minute

// DefaultAuthRefreshTTL is the rotation token lifetime: long enough for a
// school week, revocable server-side on logout/password rotation.
const DefaultAuthRefreshTTL = 7 * 24 * time.Hour

// DefaultRecoveryTTL is the password-reset token lifetime.
const DefaultRecoveryTTL = time.Hour

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

// BootstrapOwner is the central administrator created at boot: the
// developers' account in production. It receives the owner role, which
// is the only one allowed to delete accounts or touch other admins.
// Same shape and env-file conventions as BootstrapAdmin, with the
// BOOTSTRAP_OWNER_ prefix. Setting both creates both accounts.
type BootstrapOwner struct {
	Username string
	Email    string
	Password string
}

// Enabled reports whether an owner bootstrap was requested.
func (b BootstrapOwner) Enabled() bool {
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
	// TrustedProxies lists IPs/CIDRs (comma-separated) whose
	// X-Forwarded-For the throttle trusts for client IP. Empty (default)
	// trusts none: direct RemoteAddr is throttled. Set to the reverse
	// proxy (e.g. compose nginx) when the API sits behind one.
	TrustedProxies string
	// Env is the deployment shape (development by default).
	Env AppEnv
	// SeedDemo creates the demo school at boot when Env is development.
	// Defaults to true in development, forced false in production.
	SeedDemo bool
	// Bootstrap is the optional first admin account.
	Bootstrap BootstrapAdmin
	// BootstrapOwner is the optional central administrator account.
	BootstrapOwner BootstrapOwner
	// UserRetentionDays is how many days a soft-deleted account survives
	// before the purge removes it for good. Zero means the default (7).
	UserRetentionDays int
	// AuthSecret signs login tokens. Required in production; in
	// development an empty value means a random per-boot secret (logins
	// die with the process, never with a shipped default).
	AuthSecret string
	// AuthTTL is the legacy login token lifetime (access fallback).
	// Prefer AUTH_ACCESS_TTL/AUTH_REFRESH_TTL. Zero means the default.
	AuthTTL time.Duration
	// AuthAccessTTL is the API bearer lifetime. Zero means the default
	// (15m) or AUTH_TTL when set.
	AuthAccessTTL time.Duration
	// AuthRefreshTTL is the rotation token lifetime. Zero means the
	// default (7d).
	AuthRefreshTTL time.Duration
	// AuthRecoveryTTL is the password-reset token lifetime. Zero means
	// the default (1h).
	AuthRecoveryTTL time.Duration
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
	cfg.TrustedProxies = strings.TrimSpace(os.Getenv("TRUSTED_PROXIES"))
	if _, err := ParseTrustedProxies(cfg.TrustedProxies); err != nil {
		return Config{}, fmt.Errorf("config: TRUSTED_PROXIES: %w", err)
	}

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
	cfg.BootstrapOwner = BootstrapOwner{
		Username: strings.TrimSpace(os.Getenv("BOOTSTRAP_OWNER_USERNAME")),
		Email:    strings.TrimSpace(os.Getenv("BOOTSTRAP_OWNER_EMAIL")),
	}
	ownerPassword := os.Getenv("BOOTSTRAP_OWNER_PASSWORD")
	if file := strings.TrimSpace(os.Getenv("BOOTSTRAP_OWNER_PASSWORD_FILE")); file != "" {
		secret, err := readSecretFile(file)
		if err != nil {
			return Config{}, err
		}
		ownerPassword = secret
	}
	cfg.BootstrapOwner.Password = strings.TrimRight(ownerPassword, "\r\n")
	if cfg.BootstrapOwner.Username != "" && cfg.BootstrapOwner.Password == "" {
		return Config{}, fmt.Errorf(
			"config: BOOTSTRAP_OWNER_USERNAME set without BOOTSTRAP_OWNER_PASSWORD")
	}
	if cfg.BootstrapOwner.Username == "" && cfg.BootstrapOwner.Password != "" {
		return Config{}, fmt.Errorf(
			"config: BOOTSTRAP_OWNER_PASSWORD set without BOOTSTRAP_OWNER_USERNAME")
	}
	if raw := strings.TrimSpace(os.Getenv("USER_RETENTION_DAYS")); raw != "" {
		days, err := strconv.Atoi(raw)
		if err != nil || days < 1 {
			return Config{}, fmt.Errorf(
				"config: USER_RETENTION_DAYS must be a positive number of days")
		}
		cfg.UserRetentionDays = days
	}
	cfg.AuthSecret = os.Getenv("AUTH_SECRET")
	if raw := strings.TrimSpace(os.Getenv("AUTH_ACCESS_TTL")); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil || ttl <= 0 {
			return Config{}, fmt.Errorf("config: AUTH_ACCESS_TTL: %q is not a positive duration", raw)
		}
		cfg.AuthAccessTTL = ttl
	}
	if raw := strings.TrimSpace(os.Getenv("AUTH_REFRESH_TTL")); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil || ttl <= 0 {
			return Config{}, fmt.Errorf("config: AUTH_REFRESH_TTL: %q is not a positive duration", raw)
		}
		cfg.AuthRefreshTTL = ttl
	}
	if raw := strings.TrimSpace(os.Getenv("AUTH_RECOVERY_TTL")); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil || ttl <= 0 {
			return Config{}, fmt.Errorf("config: AUTH_RECOVERY_TTL: %q is not a positive duration", raw)
		}
		cfg.AuthRecoveryTTL = ttl
	}
	if raw := strings.TrimSpace(os.Getenv("AUTH_TTL")); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil || ttl <= 0 {
			return Config{}, fmt.Errorf("config: AUTH_TTL: %q is not a positive duration", raw)
		}
		cfg.AuthTTL = ttl
	}
	// Legacy fallback: AUTH_TTL configures access when the split vars are absent.
	if cfg.AuthAccessTTL == 0 && cfg.AuthTTL != 0 {
		cfg.AuthAccessTTL = cfg.AuthTTL
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
	if c.BootstrapOwner.Enabled() && isDevPassword(c.BootstrapOwner.Password) {
		return fmt.Errorf(
			"config: BOOTSTRAP_OWNER_PASSWORD is a known development password; " +
				"pick a unique one before deploying")
	}
	if strings.TrimSpace(c.AuthSecret) == "" {
		return fmt.Errorf("config: AUTH_SECRET is required when APP_ENV=production")
	}
	return nil
}

// AccessTTL resolves the effective access lifetime.
func (c Config) AccessTTL() time.Duration {
	if c.AuthAccessTTL != 0 {
		return c.AuthAccessTTL
	}
	if c.AuthTTL != 0 {
		return c.AuthTTL
	}
	return DefaultAuthAccessTTL
}

// RefreshTTL resolves the effective refresh lifetime.
func (c Config) RefreshTTL() time.Duration {
	if c.AuthRefreshTTL != 0 {
		return c.AuthRefreshTTL
	}
	return DefaultAuthRefreshTTL
}

// RecoveryTTL resolves the effective password-reset lifetime.
func (c Config) RecoveryTTL() time.Duration {
	if c.AuthRecoveryTTL != 0 {
		return c.AuthRecoveryTTL
	}
	return DefaultRecoveryTTL
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
