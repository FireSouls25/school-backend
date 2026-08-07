package main

import (
	"log/slog"
	"os"

	"grade/src/core/roles"
	"grade/src/platform/config"
	"grade/src/platform/http"
	"grade/src/platform/i18n"
)

func main() {
	cfg := config.FromEnv()

	i18nSvc, err := i18n.NewService()
	if err != nil {
		slog.Error("i18n initialization failed", "error", err)
		os.Exit(1)
	}

	store := roles.NewMemoryStore()
	roles.NewService(store)

	addr := ":" + cfg.Port
	slog.Info("starting server", "addr", addr)
	if err := httpapi.Run(addr, httpapi.Router(i18nSvc)); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
