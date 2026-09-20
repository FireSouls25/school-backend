package main

import (
	"context"
	"log/slog"
	"os"

	"grade/src/core/attendance"
	"grade/src/core/incidents"
	"grade/src/core/roles"
	"grade/src/core/students"
	"grade/src/core/subjects"
	"grade/src/core/teachers"
	"grade/src/core/warnings"
	"grade/src/platform/config"
	httpapi "grade/src/platform/http"
	"grade/src/platform/i18n"
	pg "grade/src/platform/postgres"
)

func main() {
	cfg := config.FromEnv()
	ctx := context.Background()

	i18nSvc, err := i18n.NewService()
	if err != nil {
		slog.Error("i18n initialization failed", "error", err)
		os.Exit(1)
	}

	var (
		studentsStore   students.Store   = students.NewMemoryStore()
		attendanceStore attendance.Store = attendance.NewMemoryStore()
		incidentsStore  incidents.Store  = incidents.NewMemoryStore()
		warningsStore   warnings.Store   = warnings.NewMemoryStore()
		teachersStore   teachers.Store   = teachers.NewMemoryStore()
		subjectsStore   subjects.Store   = subjects.NewMemoryStore()
	)
	if cfg.DatabaseURL != "" {
		db, err := pg.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			slog.Error("postgres connection failed", "error", err)
			os.Exit(1)
		}
		defer db.Close()
		studentsStore = pg.NewStudentsStore(db)
		attendanceStore = pg.NewAttendanceStore(db)
		incidentsStore = pg.NewIncidentsStore(db)
		warningsStore = pg.NewWarningsStore(db)
		teachersStore = pg.NewTeachersStore(db)
		subjectsStore = pg.NewSubjectsStore(db)
		slog.Info("using postgres persistence")
	} else {
		slog.Warn("DATABASE_URL not set; using in-memory stores (development only)")
	}

	_ = students.NewService(studentsStore)
	_ = attendance.NewService(attendanceStore)
	_ = incidents.NewService(incidentsStore)
	_ = warnings.NewService(warningsStore)
	_ = teachers.NewService(teachersStore)
	_ = subjects.NewService(subjectsStore)

	roleStore := roles.NewMemoryStore()
	roles.NewService(roleStore)

	addr := ":" + cfg.Port
	slog.Info("starting server", "addr", addr)
	if err := httpapi.Run(addr, httpapi.Router(i18nSvc)); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
