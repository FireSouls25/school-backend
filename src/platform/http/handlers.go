package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"grade/src/core/roles"
	"grade/src/core/sessions"
	"grade/src/core/statistics"
	"grade/src/core/students"
	"grade/src/core/teachers"
	"grade/src/core/warnings"
)

// errBadRequest renders 400 when the request body is unreadable.
var errBadRequest = codedError{"http.err_bad_request"}

// maxBodyBytes caps JSON request bodies (1 MiB).
const maxBodyBytes = 1 << 20

// Dependencies wires handlers to core services. Only the composition root
// builds it.
type Dependencies struct {
	Auth       roles.Authorizer
	Roles      roles.RoleGetter
	RolesSvc   *roles.Service
	Students   *students.Service
	Teachers   *teachers.Service
	Warnings   *warnings.Service
	Sessions   *sessions.Service
	Statistics *statistics.Service
	// AllowedOrigins lists browser origins accepted by CORS.
	AllowedOrigins []string
}

// meResponse describes the caller for frontend navigation (roles, home).
type meResponse struct {
	Subject string       `json:"subject"`
	Roles   []roles.Role `json:"roles"`
	Home    string       `json:"home"`
}

// homeFor maps the highest-priority role to the frontend home path.
// The backend suggests; the SPA owns its routes.
func homeFor(rs []roles.Role) string {
	set := make(map[roles.Role]bool, len(rs))
	for _, r := range rs {
		set[r] = true
	}
	switch {
	case set[roles.RoleAdmin]:
		return "/admin"
	case set[roles.RoleTeacher]:
		return "/docente"
	case set[roles.RoleStudent]:
		return "/estudiante"
	default:
		return "/"
	}
}

// handleMe returns the caller identity, roles and suggested home.
func handleMe(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, _ := SubjectFrom(r.Context())
		rs, err := deps.Roles.Roles(r.Context(), subject)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if rs == nil {
			rs = []roles.Role{}
		}
		writeJSON(w, http.StatusOK, meResponse{Subject: subject, Roles: rs, Home: homeFor(rs)})
	}
}

// allowStudentView reports whether the caller may see studentID: teachers
// and admins via view-students, students only via view-own-history on
// their own id. Authorizer failures deny.
func allowStudentView(ctx context.Context, auth roles.Authorizer, subject, studentID string) bool {
	if ok, err := auth.Can(ctx, subject, roles.PermissionViewStudents); err == nil && ok {
		return true
	}
	if subject != studentID {
		return false
	}
	ok, err := auth.Can(ctx, subject, roles.PermissionViewOwnHistory)
	return err == nil && ok
}

// handleGetStudent returns one student profile.
func handleGetStudent(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, _ := SubjectFrom(r.Context())
		id := chi.URLParam(r, "id")
		if !allowStudentView(r.Context(), deps.Auth, subject, id) {
			WriteError(w, r, errForbidden)
			return
		}
		st, err := deps.Students.ByID(r.Context(), id)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, st)
	}
}

// handleGetStudentWarnings returns one student's warning history.
func handleGetStudentWarnings(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, _ := SubjectFrom(r.Context())
		id := chi.URLParam(r, "id")
		if !allowStudentView(r.Context(), deps.Auth, subject, id) {
			WriteError(w, r, errForbidden)
			return
		}
		list, err := deps.Warnings.ForStudent(r.Context(), id)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// handleClassReport returns the per-class-group statistics report.
func handleClassReport(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report, err := deps.Statistics.ClassReport(r.Context(), chi.URLParam(r, "groupID"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
	}
}

// handleStudentReport returns the full cross-year student report.
func handleStudentReport(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report, err := deps.Statistics.StudentReport(r.Context(), chi.URLParam(r, "studentID"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
	}
}

// markRequest is the body for recording a roll-call mark.
type markRequest struct {
	StudentID string `json:"studentID"`
	Mark      string `json:"mark"`
	Note      string `json:"note"`
}

// decodeJSON parses a bounded, strict JSON body.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		WriteError(w, r, errBadRequest)
		return false
	}
	if dec.More() {
		WriteError(w, r, errBadRequest)
		return false
	}
	return true
}

// handleRecordMark records one roll-call mark; the author is always the
// authenticated caller, never a body field.
func handleRecordMark(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body markRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		// Empty mark clears back to present (correction flow).
		var mark sessions.Mark
		if raw := strings.TrimSpace(body.Mark); raw != "" {
			m, err := sessions.Parse(raw)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			mark = m
		}
		subject, _ := SubjectFrom(r.Context())
		rev, err := deps.Sessions.RecordMark(
			r.Context(), chi.URLParam(r, "sessionID"),
			strings.TrimSpace(body.StudentID), mark, subject,
			strings.TrimSpace(body.Note),
		)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, rev)
	}
}

// handleCreateStudent persists a new student profile. The service assigns
// the id and forces active status; any incoming ID or Status is ignored.
func handleCreateStudent(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body students.Student
		if !decodeJSON(w, r, &body) {
			return
		}
		st, err := deps.Students.Create(r.Context(), body)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, st)
	}
}

// handleCreateTeacher persists a new teacher profile. The service assigns
// the id and forces active status; any incoming ID or Active is ignored.
// Login identity for the new teacher is granted separately through
// POST /v1/roles until users/auth lands.
func handleCreateTeacher(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body teachers.Teacher
		if !decodeJSON(w, r, &body) {
			return
		}
		tc, err := deps.Teachers.Create(r.Context(), body)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, tc)
	}
}

// roleAssignment is the body for granting a role to a subject.
type roleAssignment struct {
	SubjectID string `json:"subjectID"`
	Role      string `json:"role"`
}

// roleGrant is the response after granting a role.
type roleGrant struct {
	SubjectID string       `json:"subjectID"`
	Roles     []roles.Role `json:"roles"`
}

// handleAssignRole grants a role to a subject. Role parsing validates the
// value; unknown roles fail with roles.err_unknown_role.
func handleAssignRole(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body roleAssignment
		if !decodeJSON(w, r, &body) {
			return
		}
		role, err := roles.Parse(body.Role)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		subjectID := strings.TrimSpace(body.SubjectID)
		if err := deps.RolesSvc.Assign(r.Context(), subjectID, role); err != nil {
			WriteError(w, r, err)
			return
		}
		rs, err := deps.Roles.Roles(r.Context(), subjectID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, roleGrant{SubjectID: subjectID, Roles: rs})
	}
}

// handleListTeachers returns every teacher profile, alphabetically.
func handleListTeachers(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := deps.Teachers.List(r.Context())
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// handleGetTeacher returns one teacher profile.
func handleGetTeacher(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tc, err := deps.Teachers.ByID(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, tc)
	}
}
