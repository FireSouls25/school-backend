// Package httpapi wires the HTTP transport: routes, middleware and responses.
// It depends on core features only through their ports (e.g. Authorizer).
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"grade/src/platform/i18n"
)

// Router builds the application's HTTP routes.
func Router(i18nSvc *i18n.Service) http.Handler {
	r := chi.NewRouter()
	r.Use(MiddlewareLocale(i18nSvc))
	r.Get("/healthz", handleHealthz)
	return r
}

// handleHealthz reports service liveness.
func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Run starts the HTTP server and blocks until SIGINT or SIGTERM, then shuts
// down gracefully.
func Run(addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
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
