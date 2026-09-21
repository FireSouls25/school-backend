package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"grade/src/core/roles"
)

// MiddlewareCORS reflects only explicitly allowed origins, so browser
// clients on other origins stay blocked while same-origin and non-browser
// clients (Tauri, curl) pass through untouched.
func MiddlewareCORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || !allowed[origin] {
				if r.Method == http.MethodOptions && origin != "" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Subject-ID")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// routes registers the versioned API. Conventions, kept consistent:
//
//   - /healthz is public; everything under /v1 requires a subject.
//   - Reads declare one required permission; self-scoped student reads
//     additionally accept view-own-history on the caller's own id.
//   - The full cross-year student report is admin-only (manage-system);
//     teachers read class-scoped data (view-students, view-class-statistics).
//   - Writes declare the acting permission; the author is always the
//     authenticated caller, never a body field.
//   - Denials never leak data and always carry a stable code:
//     http.err_unauthorized (401) or http.err_forbidden (403). The SPA
//     navigates back on 403, defaulting to the GET /v1/me home.
func routes(r *chi.Mux, deps Dependencies) {
	r.Get("/healthz", handleHealthz)

	r.Route("/v1", func(r chi.Router) {
		r.Use(MiddlewareSubject)
		r.Get("/me", handleMe(deps))

		r.Get("/students/{id}", handleGetStudent(deps))
		r.Get("/students/{id}/warnings", handleGetStudentWarnings(deps))

		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/statistics/class/{groupID}", handleClassReport(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageSystem)).
			Get("/statistics/student/{studentID}", handleStudentReport(deps))

		r.With(RequirePermission(deps.Auth, roles.PermissionRecordAttendance)).
			Post("/sessions/{sessionID}/marks", handleRecordMark(deps))

		r.With(RequirePermission(deps.Auth, roles.PermissionManageStudents)).
			Post("/students", handleCreateStudent(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Post("/teachers", handleCreateTeacher(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Get("/teachers", handleListTeachers(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Get("/teachers/{id}", handleGetTeacher(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageRoles)).
			Post("/roles", handleAssignRole(deps))
	})
}
