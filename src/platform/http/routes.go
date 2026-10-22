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

	r.Post("/v1/login", handleLogin(deps, newLoginLimiter(deps.Throttle, deps.TrustedProxies, loginLimitPerMinute, loginWindow)))
	r.Post("/v1/refresh", handleRefresh(deps, newLoginLimiter(deps.Throttle, deps.TrustedProxies, refreshLimitPerMinute, loginWindow)))
	// Logout revokes the presented refresh (cookie/body) or, with all=true
	// plus an access bearer, every session of the caller. No subject is
	// required for single logout: the refresh token itself authenticates.
	r.Post("/v1/logout", handleLogout(deps))
	// Password recovery: always-204 request (no enumeration), single-use
	// reset. Throttled per IP like login.
	r.Post("/v1/recover/request", handleRecoverRequest(deps, newLoginLimiter(deps.Throttle, deps.TrustedProxies, recoverLimitPerMinute, loginWindow)))
	r.Post("/v1/recover/reset", handleRecoverReset(deps, newLoginLimiter(deps.Throttle, deps.TrustedProxies, recoverLimitPerMinute, loginWindow)))

	r.Route("/v1", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return MiddlewareSubjectWith(next, deps.Tokens, deps.AllowDevIdentity)
		})
		// Version-based revocation: rotated/disabled accounts fail closed.
		r.Use(RequireTokenFresh(deps.Users))
		r.Get("/me", handleMe(deps))
		// Self-service password: auth only, the current password gates
		// the swap, so permissions play no part here.
		r.Put("/me/password", handleChangeMyPassword(deps))

		r.Get("/students/{id}", handleGetStudent(deps))
		r.Get("/students/{id}/warnings", handleGetStudentWarnings(deps))
		r.Get("/students/{id}/incidents", handleStudentIncidents(deps))

		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/classes", handleListClasses(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Post("/classes", handleCreateClass(deps))
		// Enrollments and promotion decisions (manage-students).
		r.With(RequirePermission(deps.Auth, roles.PermissionManageStudents)).
			Post("/enrollments", handleCreateEnrollment(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageStudents)).
			Get("/enrollments", handleListEnrollments(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageStudents)).
			Delete("/enrollments/{id}", handleDeleteEnrollment(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageStudents)).
			Post("/promotions", handleRecordPromotion(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageStudents)).
			Get("/promotions", handleListPromotions(deps))
		// School years: dated periods, frozen calendars, close.
		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/schoolyears", handleListYears(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/schoolyears/{id}", handleGetYear(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Post("/schoolyears", handleCreateYear(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Put("/schoolyears/{id}", handleUpdateYear(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Post("/schoolyears/{id}/close", handleCloseYear(deps))
		// Official Colombian holidays as an editable seed for the year form.
		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/holidays", handleHolidays(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/classes/{groupID}/roster", handleClassRoster(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageSystem)).
			Get("/dashboard/summary", handleDashboardSummary(deps))

		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/statistics/class/{groupID}", handleClassReport(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageSystem)).
			Get("/statistics/student/{studentID}", handleStudentReport(deps))

		r.With(RequirePermission(deps.Auth, roles.PermissionRecordAttendance)).
			Post("/sessions/{sessionID}/marks", handleRecordMark(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionRecordAttendance)).
			Post("/sessions/open", handleOpenSession(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/sessions/history", handleSessionsHistory(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/sessions/{id}", handleSessionDetail(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionRecordIncidents)).
			Post("/warnings/batch", handleIssueWarningBatch(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionRecordIncidents)).
			Put("/warnings/{id}", handleCorrectWarning(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionRecordIncidents)).
			Get("/warnings/{id}/revisions", handleWarningRevisions(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageSystem)).
			Get("/warnings/recent", handleWarningsRecent(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionRecordIncidents)).
			Post("/incidents", handleReportIncident(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageSystem)).
			Get("/incidents/recent", handleIncidentsRecent(deps))

		r.With(RequirePermission(deps.Auth, roles.PermissionManageStudents)).
			Post("/students", handleCreateStudent(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageStudents)).
			Put("/students/{id}", handleUpdateStudent(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageStudents)).
			Put("/students/{id}/photo", handlePutPhoto(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionViewStudents)).
			Get("/students/{id}/photo", handleGetPhoto(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Post("/teachers", handleCreateTeacher(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Put("/teachers/{id}", handleUpdateTeacher(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Get("/teachers", handleListTeachers(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/teachers/me/schedule", handleMySchedule(deps))
		// Self-service photo: the caller's own teacher profile. Auth only,
		// no permission — it can only resolve to the caller's own image.
		r.Get("/teachers/me/photo", handleGetMyTeacherPhoto(deps))
		r.Put("/teachers/me/photo", handlePutMyTeacherPhoto(deps))
		// Account-level twins: linked docentes keep writing through to
		// their profile; every other login stores the picture inside its
		// own account row. Auth only, no permission.
		r.Get("/me/photo", handleGetMyPhoto(deps))
		r.Put("/me/photo", handlePutMyPhoto(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Get("/teachers/{id}", handleGetTeacher(deps))
		// Lockout UI: which login (if any) fronts a teacher profile.
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Get("/teachers/{id}/account", handleGetTeacherAccount(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Put("/teachers/{id}/photo", handlePutTeacherPhoto(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Get("/teachers/{id}/photo", handleGetTeacherPhoto(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageTeachers)).
			Post("/teachers/attendance", handleRecordTeacherDay(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageTeachers)).
			Get("/teachers/attendance", handleTeacherDay(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageTeachers)).
			Get("/teachers/{id}/attendance", handleTeacherHistory(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageTeachers)).
			Get("/teachers/{id}/sessions", handleTeacherSessions(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageTeachers)).
			Get("/teachers/{id}/warnings-made", handleTeacherWarningsMade(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageTeachers)).
			Get("/teachers/roll", handleTeacherRoll(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageTeachers)).
			Get("/teachers/attendance/history", handleTeacherAttendanceRange(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageTeachers)).
			Get("/teachers/attendance/revisions", handleTeacherDayRevisions(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/subjects", handleListSubjects(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/subjects/{id}", handleGetSubject(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Post("/subjects", handleCreateSubject(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Put("/subjects/{id}", handleUpdateSubject(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Delete("/subjects/{id}", handleDeleteSubject(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/subjects/{id}/assignments", handleSubjectAssignments(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Post("/assignments", handleAssignSubject(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Post("/assignments/{id}/end", handleEndAssignment(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Delete("/assignments/{id}", handleRemoveAssignment(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Get("/teachers/{id}/assignments", handleTeacherAssignments(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionViewClassStats)).
			Get("/schedule", handleListSchedule(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Post("/schedule", handleCreateSchedule(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Put("/schedule/{id}", handleUpdateSchedule(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Delete("/schedule/{id}", handleDeleteSchedule(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageSystem)).
			Get("/audit", handleListAudit(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageRoles)).
			Post("/roles", handleAssignRole(deps))
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Post("/users", handleCreateUser(deps))
		// Lockout response: disable/enable a login without deleting it.
		// The version bump kills bearers; refresh is rejected on Active.
		r.With(RequirePermission(deps.Auth, roles.PermissionManageUsers)).
			Put("/users/{id}/active", handleSetUserActive(deps))
	})
}
