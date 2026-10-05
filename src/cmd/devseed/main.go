// Command devseed creates the demo school used for development and
// manual testing. It is idempotent: running it twice changes nothing.
//
// Usage:
//
//	go run ./src/cmd/devseed            # seed DATABASE_URL
//	SEED_DEMO=0 go run ./src/cmd/server # the server seeds on boot too
//
// Demo logins: admin / admin123* and carlos.mendoza / docente123*.
// These are development-only: APP_ENV=production refuses demo data and
// refuses these passwords for the bootstrap account.
package main

import (
	"context"
	"log/slog"
	"os"

	"grade/src/app"
	"grade/src/platform/config"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	if cfg.Env.IsProduction() {
		slog.Error("devseed refused: APP_ENV=production", "hint",
			"demo data must never be created in production")
		os.Exit(1)
	}
	ctx := context.Background()

	application, err := app.New(ctx, cfg)
	if err != nil {
		slog.Error("application wiring failed", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	sum, err := application.SeedDemo(ctx)
	if err != nil {
		slog.Error("seeding failed", "error", err)
		os.Exit(1)
	}
	if sum.AlreadySeeded {
		slog.Info(sum.String())
		return
	}
	slog.Info(sum.String(), "records", sum.Total(),
		"login", "admin", "password", "admin123*")
}
