// Package httpapi wires the HTTP transport: routes, middleware and responses.
// It depends on core features only through their ports (e.g. Authorizer).
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"grade/src/platform/i18n"
)

// Router builds the application's HTTP routes.
func Router(deps Dependencies, i18nSvc *i18n.Service) http.Handler {
	r := chi.NewRouter()
	r.Use(MiddlewareLocale(i18nSvc))
	r.Use(MiddlewareCORS(deps.AllowedOrigins))
	r.Use(MiddlewareAccessLog)
	routes(r, deps)
	return r
}

// statusRecorder captures the status code for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// MiddlewareAccessLog logs one line per request (method, path, status,
// latency) at info level. It carries no bodies, tokens or notes: bodies
// stay out of the log by design (Ley 1581: student data minimization).
// /healthz is skipped: orchestrator probes would drown real traffic.
func MiddlewareAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"latency_ms", time.Since(start).Milliseconds(),
		)
	})
}

// handleHealthz reports service liveness plus the build marker. Empty
// versions read as dev so tests and local runs need no configuration.
// recoveryMail tells the console whether reset links actually leave the
// server, so it warns instead of promising undeliverable mail.
func handleHealthz(version string, recoveryMail bool) http.HandlerFunc {
	if version == "" {
		version = "dev"
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": version, "recoveryMail": recoveryMail})
	}
}

// Run starts the HTTP server and blocks until SIGINT or SIGTERM, then shuts
// down gracefully.
func Run(addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	select {
	case err := <-errCh:
		return err
	case <-stop:
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}
