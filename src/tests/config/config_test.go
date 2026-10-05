package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grade/src/platform/config"
)

// clearEnv unsets every variable the config reads so each test starts
// from a known state regardless of the developer's shell.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"PORT", "DATABASE_URL", "ALLOWED_ORIGINS", "APP_ENV", "SEED_DEMO",
		"BOOTSTRAP_ADMIN_USERNAME", "BOOTSTRAP_ADMIN_EMAIL",
		"BOOTSTRAP_ADMIN_PASSWORD", "BOOTSTRAP_ADMIN_PASSWORD_FILE",
	} {
		t.Setenv(key, "")
	}
}

func TestDefault(t *testing.T) {
	cfg := config.Default()
	if cfg.Port != "8080" {
		t.Errorf("Default().Port = %q, want %q", cfg.Port, "8080")
	}
	if cfg.Env != config.EnvDevelopment {
		t.Errorf("Default().Env = %q, want development", cfg.Env)
	}
	if !cfg.SeedDemo {
		t.Error("Default().SeedDemo = false, want true in development")
	}
}

func TestFromEnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("PORT", "9090")
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("FromEnv().Port = %q, want %q", cfg.Port, "9090")
	}
}

func TestFromEnvFallback(t *testing.T) {
	clearEnv(t)
	t.Setenv("PORT", "")
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("FromEnv().Port = %q, want %q", cfg.Port, "8080")
	}
}

func TestOriginsAndDatabase(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/gradedb")
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:1420, http://127.0.0.1:1420 ,")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost/gradedb" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Fatalf("AllowedOrigins = %v, want 2 entries (blank dropped)", cfg.AllowedOrigins)
	}
	if cfg.AllowedOrigins[0] != "http://localhost:1420" ||
		cfg.AllowedOrigins[1] != "http://127.0.0.1:1420" {
		t.Errorf("AllowedOrigins = %v", cfg.AllowedOrigins)
	}
	if cfg.UsesMemoryStores() {
		t.Error("UsesMemoryStores() = true with DATABASE_URL set")
	}
}

func TestProductionDisablesDemoData(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://localhost/gradedb")
	t.Setenv("SEED_DEMO", "true")

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.Env.IsProduction() != true {
		t.Fatalf("Env = %q, want production", cfg.Env)
	}
	if cfg.SeedDemo {
		t.Error("SeedDemo = true in production; demo data must be impossible to enable")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate = %v, want nil", err)
	}
}

func TestValidateRejectsUnsafeProduction(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{
			name: "no database in production",
			cfg:  config.Config{Env: config.EnvProduction, SeedDemo: false},
			want: "DATABASE_URL is required",
		},
		{
			name: "demo data in production",
			cfg: config.Config{
				Env:         config.EnvProduction,
				DatabaseURL: "postgres://localhost/gradedb",
				SeedDemo:    true,
			},
			want: "SEED_DEMO cannot be enabled",
		},
		{
			name: "known development password",
			cfg: config.Config{
				Env:         config.EnvProduction,
				DatabaseURL: "postgres://localhost/gradedb",
				Bootstrap:   config.BootstrapAdmin{Username: "admin", Password: "admin123*"},
			},
			want: "known development password",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want error mentioning %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Validate() = %v, want error mentioning %q", err, tc.want)
			}
		})
	}
}

func TestDevelopmentValidateAlwaysPasses(t *testing.T) {
	clearEnv(t)
	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil in development", err)
	}
}

func TestBootstrapFromFile(t *testing.T) {
	clearEnv(t)
	secret := filepath.Join(t.TempDir(), "bootstrap_password")
	if err := os.WriteFile(secret, []byte("s3cret-from-file\n"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	t.Setenv("BOOTSTRAP_ADMIN_USERNAME", "directora")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD_FILE", secret)

	cfg, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if !cfg.Bootstrap.Enabled() {
		t.Fatal("Bootstrap.Enabled() = false, want true")
	}
	// Only the trailing newline is stripped; the secret stays intact.
	if cfg.Bootstrap.Password != "s3cret-from-file" {
		t.Errorf("Bootstrap.Password = %q, want %q", cfg.Bootstrap.Password, "s3cret-from-file")
	}
}

func TestBootstrapMissingFileFails(t *testing.T) {
	clearEnv(t)
	t.Setenv("BOOTSTRAP_ADMIN_USERNAME", "directora")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD_FILE", filepath.Join(t.TempDir(), "absent"))

	if _, err := config.FromEnv(); err == nil {
		t.Error("FromEnv() = nil error, want failure for a missing secret file")
	}
}

func TestHalfConfiguredBootstrapFails(t *testing.T) {
	clearEnv(t)
	t.Setenv("BOOTSTRAP_ADMIN_USERNAME", "directora")
	if _, err := config.FromEnv(); err == nil {
		t.Error("FromEnv() = nil error, want failure when the password is missing")
	}

	clearEnv(t)
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "s3cret-pass1")
	if _, err := config.FromEnv(); err == nil {
		t.Error("FromEnv() = nil error, want failure when the username is missing")
	}
}

func TestMalformedValuesAreRejected(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "staging")
	if _, err := config.FromEnv(); err == nil {
		t.Error("FromEnv() accepted APP_ENV=staging; unknown environments must fail loudly")
	}

	clearEnv(t)
	t.Setenv("SEED_DEMO", "maybe")
	if _, err := config.FromEnv(); err == nil {
		t.Error("FromEnv() accepted SEED_DEMO=maybe; only booleans are allowed")
	}
}

func TestEnvironmentAliases(t *testing.T) {
	for raw, want := range map[string]config.AppEnv{
		"":           config.EnvDevelopment,
		"dev":        config.EnvDevelopment,
		"production": config.EnvProduction,
		"PROD":       config.EnvProduction,
	} {
		clearEnv(t)
		t.Setenv("APP_ENV", raw)
		cfg, err := config.FromEnv()
		if err != nil {
			t.Fatalf("FromEnv(%q): %v", raw, err)
		}
		if cfg.Env != want {
			t.Errorf("APP_ENV %q -> %q, want %q", raw, cfg.Env, want)
		}
	}
}
