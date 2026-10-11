package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"grade/src/core/audit"
)

// statusRecorder remembers the status code a handler ended with, so the
// audit trail can store how the access ended (a denied read stays visible).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// MiddlewareAudit records every access to student data: who did it, what they
// touched and how the request ended (Ley 1581 de 2012).
//
// It classifies the matched route, never the raw URL, so classification stays
// stable when paths change. Writing the trail is best effort: a broken audit
// store is logged, but it never turns a working request into a failure.
func MiddlewareAudit(svc *audit.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			action, resource, resourceID, auditable := classify(r)
			if !auditable {
				next.ServeHTTP(w, r)
				return
			}
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			defer func() {
				subject, ok := SubjectFrom(r.Context())
				if !ok {
					return // unauthenticated requests carry no author
				}
				if _, err := svc.Record(r.Context(), subject, action, resource, resourceID, rec.status); err != nil {
					slog.Warn("audit entry not stored", "subject", subject, "resource", string(resource), "error", err)
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// classify maps a matched route to an audited action. It returns auditable
// false for routes that touch no student data (login, teachers, catalogs).
func classify(r *http.Request) (audit.Action, audit.Resource, string, bool) {
	pattern := chi.RouteContext(r.Context()).RoutePattern()
	id := chi.URLParam(r, "id")
	studentID := chi.URLParam(r, "studentID")

	switch pattern {
	case "/v1/students":
		if r.Method == http.MethodPost {
			return audit.ActionCreate, audit.ResourceStudent, "", true
		}
	case "/v1/students/{id}":
		if r.Method == http.MethodGet {
			return audit.ActionRead, audit.ResourceStudent, id, true
		}
	case "/v1/students/{id}/warnings":
		if r.Method == http.MethodGet {
			return audit.ActionRead, audit.ResourceWarning, id, true
		}
	case "/v1/sessions/{sessionID}/marks":
		if r.Method == http.MethodPost {
			// The mark belongs to a student named in the body; the session id
			// keeps the trail traceable without parsing the payload.
			return audit.ActionMark, audit.ResourceSession, chi.URLParam(r, "sessionID"), true
		}
	case "/v1/statistics/student/{studentID}":
		if r.Method == http.MethodGet {
			return audit.ActionRead, audit.ResourceReport, studentID, true
		}
	case "/v1/statistics/class/{groupID}":
		if r.Method == http.MethodGet {
			return audit.ActionRead, audit.ResourceReport, chi.URLParam(r, "groupID"), true
		}
	}
	return "", "", "", false
}

// auditLimit reads the ?limit= parameter for the audit listing endpoint.
func auditLimit(r *http.Request, def int) int {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
