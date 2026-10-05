// Command server runs the grade REST API.
package main

import (
	"context"
	"log/slog"
	"os"

	"grade/src/app"
	"grade/src/platform/config"
	httpapi "grade/src/platform/http"
	"grade/src/platform/i18n"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	ctx := context.Background()

	i18nSvc, err := i18n.NewService()
	if err != nil {
		slog.Error("i18n initialization failed", "error", err)
		os.Exit(1)
	}

	application, err := app.New(ctx, cfg)
	if err != nil {
		slog.Error("application wiring failed", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	// Legacy placeholder identities (X-Subject-ID), when configured.
	application.SeedPlaceholderRoles(ctx)
	// Bootstrap admin + demo data (development only), idempotent.
	if err := application.Prepare(ctx); err != nil {
		slog.Error("startup preparation failed", "error", err)
		os.Exit(1)
	}

	addr := ":" + cfg.Port
	slog.Info("starting server",
		"addr", addr, "env", string(cfg.Env), "demo_data", cfg.SeedDemo)
	if err := httpapi.Run(addr, httpapi.Router(httpapi.Dependencies{
		Auth:           application.Services.Roles,
		Roles:          application.Services.Roles,
		RolesSvc:       application.Services.Roles,
		Students:       application.Services.Students,
		Teachers:       application.Services.Teachers,
		Users:          application.Services.Users,
		Warnings:       application.Services.Warnings,
		Sessions:       application.Services.Sessions,
		Statistics:     application.Services.Statistics,
		AllowedOrigins: cfg.AllowedOrigins,
	}, i18nSvc)); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
