// Command server runs the grade REST API.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"grade/src/app"
	"grade/src/core/throttle"
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

	// Trusted proxies for client IP (throttle + audit). Invalid values
	// already fail in config.FromEnv; re-parse here for the nets.
	trusted, err := throttle.ParseTrusted(cfg.TrustedProxies)
	if err != nil {
		slog.Error("invalid trusted proxies", "error", err)
		os.Exit(1)
	}

	// Legacy placeholder identities (X-Subject-ID), when configured.
	application.SeedPlaceholderRoles(ctx)
	// Bootstrap admin + owner + demo data (development only), idempotent.
	if err := application.Prepare(ctx); err != nil {
		slog.Error("startup preparation failed", "error", err)
		os.Exit(1)
	}
	if err := application.BootstrapOwner(ctx); err != nil {
		slog.Error("startup owner bootstrap failed", "error", err)
		os.Exit(1)
	}
	// Soft-deleted accounts outlive their deletion by the retention
	// window and then disappear for good. Purge once at startup and
	// hourly after that; a missed run only delays, never skips, expiry.
	if _, err := application.PurgeDeletedUsers(ctx); err != nil {
		slog.Error("startup purge failed", "error", err)
	}
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := application.PurgeDeletedUsers(context.Background()); err != nil {
				slog.Error("scheduled purge failed", "error", err)
			}
		}
	}()

	addr := ":" + cfg.Port
	slog.Info("starting server",
		"addr", addr, "env", string(cfg.Env), "demo_data", cfg.SeedDemo)
	if err := httpapi.Run(addr, httpapi.Router(httpapi.Dependencies{
		Auth:              application.Services.Roles,
		Roles:             application.Services.Roles,
		RolesSvc:          application.Services.Roles,
		Students:          application.Services.Students,
		Teachers:          application.Services.Teachers,
		Users:             application.Services.Users,
		Warnings:          application.Services.Warnings,
		Sessions:          application.Services.Sessions,
		Statistics:        application.Services.Statistics,
		Dashboard:         application.Services.Dashboard,
		Classes:           application.Services.Classes,
		Years:             application.Services.Years,
		Enrollments:       application.Services.Enrollments,
		Schedules:         application.Services.Schedules,
		Incidents:         application.Services.Incidents,
		Academics:         application.Services.Academics,
		Audit:             application.Services.Audit,
		Tokens:            application.Services.Auth,
		Throttle:          application.Services.Throttle,
		TrustedProxies:    trusted,
		Recovery:          application.Services.Recovery,
		RecoveryLogTokens: !cfg.Env.IsProduction(),
		AllowDevIdentity:  !cfg.Env.IsProduction(),
		Subjects:          application.Services.Subjects,
		TAttendance:       application.Services.TAttendance,
		AllowedOrigins:    cfg.AllowedOrigins,
	}, i18nSvc)); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
