// Command server runs the grade REST API.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"

	"grade/src/app"
	"grade/src/core/auth"
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

	// Token secret: production requires AUTH_SECRET (enforced by
	// Validate); development mints a random per-boot secret so no
	// default ever ships, at the cost of logins dying on restart.
	secret := cfg.AuthSecret
	if secret == "" {
		var raw [32]byte
		if _, err := rand.Read(raw[:]); err != nil {
			slog.Error("random secret failed", "error", err)
			os.Exit(1)
		}
		secret = hex.EncodeToString(raw[:])
		slog.Warn("AUTH_SECRET unset; using a random per-boot secret (development only)")
	}
	ttl := cfg.AuthTTL
	if ttl <= 0 {
		ttl = config.DefaultAuthTTL
	}
	tokens, err := auth.NewService(secret, ttl)
	if err != nil {
		slog.Error("auth wiring failed", "error", err)
		os.Exit(1)
	}

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
		Auth:             application.Services.Roles,
		Roles:            application.Services.Roles,
		RolesSvc:         application.Services.Roles,
		Students:         application.Services.Students,
		Teachers:         application.Services.Teachers,
		Users:            application.Services.Users,
		Warnings:         application.Services.Warnings,
		Sessions:         application.Services.Sessions,
		Statistics:       application.Services.Statistics,
		Dashboard:        application.Services.Dashboard,
		Classes:          application.Services.Classes,
		Years:            application.Services.Years,
		Enrollments:      application.Services.Enrollments,
		Schedules:        application.Services.Schedules,
		Incidents:        application.Services.Incidents,
		Audit:            application.Services.Audit,
		Tokens:           tokens,
		AllowDevIdentity: !cfg.Env.IsProduction(),
		Subjects:         application.Services.Subjects,
		AllowedOrigins:   cfg.AllowedOrigins,
	}, i18nSvc)); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
