package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"grade/src/core/audit"
	"grade/src/core/auth"
	"grade/src/core/classes"
	"grade/src/core/enrollments"
	"grade/src/core/incidents"
	"grade/src/core/roles"
	"grade/src/core/schedules"
	"grade/src/core/schoolyears"
	"grade/src/core/sessions"
	"grade/src/core/statistics"
	"grade/src/core/students"
	"grade/src/core/subjects"
	"grade/src/core/tattendance"
	"grade/src/core/teachers"
	"grade/src/core/users"
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
	Users      *users.Service
	Warnings   *warnings.Service
	Sessions   *sessions.Service
	Statistics *statistics.Service
	Dashboard  *statistics.DashboardService
	Audit      *audit.Service
	// Tokens mints and verifies login tokens. Nil means no bearer
	// support (tests without auth setup); the dev header still applies.
	Tokens *auth.Service
	// AllowDevIdentity keeps the X-Subject-ID fallback. True in
	// development and tests, false in production.
	AllowDevIdentity bool
	Subjects         *subjects.Service
	TAttendance      *tattendance.Service
	Classes          *classes.Service
	Years            *schoolyears.Service
	Enrollments      *enrollments.Service
	Schedules        *schedules.Service
	Incidents        *incidents.Service
	// AllowedOrigins lists browser origins accepted by CORS.
	AllowedOrigins []string
}

// recordAudit trails one mutating action with its real outcome, so
// denied attempts stay visible too. Recording never fails the action:
// a nil service (tests with partial dependencies) or a record error
// only drops the row, loudly in the server log.
func recordAudit(deps Dependencies, r *http.Request, action audit.Action, resource audit.Resource, resourceID, studentID string, status int) {
	if deps.Audit == nil {
		return
	}
	subject, _ := SubjectFrom(r.Context())
	if _, err := deps.Audit.Record(r.Context(), audit.Entry{
		SubjectID: subject, Action: action, Resource: resource,
		ResourceID: resourceID, StudentID: studentID, Status: status,
	}); err != nil {
		slog.Warn("audit record dropped", "resource", string(resource), "error", err)
	}
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

// photoRequest carries a base64 image (plain or data-URL form) for
// upload. Empty data clears the stored photo.
type photoRequest struct {
	Data string `json:"data"`
}

// photoResponse returns the stored photo as base64 with its detected
// content type, ready for a data URL.
type photoResponse struct {
	Data        string `json:"data"`
	ContentType string `json:"contentType"`
}

// errPhotoNotFound renders 404 when the student has no photo yet.
var errPhotoNotFound = codedError{"students.err_photo_not_found"}

// maxPhotoBodyBytes caps photo uploads above the 2 MiB domain limit:
// base64 inflates by 4/3, so the JSON envelope needs headroom.
const maxPhotoBodyBytes = 4 << 20

// decodePhotoBody is decodeJSON with a photo-sized body cap.
func decodePhotoBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBodyBytes)
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

// photoBytes strips an optional data-URL prefix and decodes base64.
func photoBytes(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if i := strings.Index(raw, ","); i >= 0 && strings.Contains(raw[:i], "base64") {
		raw = raw[i+1:]
	}
	return base64.StdEncoding.DecodeString(raw)
}

// handlePutPhoto stores (or clears, when empty) a student photo.
func handlePutPhoto(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body photoRequest
		if !decodePhotoBody(w, r, &body) {
			return
		}
		data, err := photoBytes(body.Data)
		if err != nil {
			WriteError(w, r, errBadRequest)
			return
		}
		if err := deps.Students.PutPhoto(r.Context(), chi.URLParam(r, "id"), students.Photo{Data: data}); err != nil {
			WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleGetPhoto returns the stored photo, if any.
func handleGetPhoto(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, _ := SubjectFrom(r.Context())
		id := chi.URLParam(r, "id")
		if !allowStudentView(r.Context(), deps.Auth, subject, id) {
			WriteError(w, r, errForbidden)
			return
		}
		photo, err := deps.Students.Photo(r.Context(), id)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if len(photo.Data) == 0 {
			WriteError(w, r, errPhotoNotFound)
			return
		}
		writeJSON(w, http.StatusOK, photoResponse{
			Data:        base64.StdEncoding.EncodeToString(photo.Data),
			ContentType: http.DetectContentType(photo.Data),
		})
	}
}

// handleUpdateStudent replaces a student profile. The id comes from
// the path and the lifecycle status is managed server-side, so both
// are ignored in the body and taken from the stored record instead.
func handleUpdateStudent(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body students.Student
		if !decodeJSON(w, r, &body) {
			return
		}
		body.ID = id
		updated, err := deps.Students.Update(r.Context(), body)
		if err != nil {
			recordAudit(deps, r, audit.ActionUpdate, audit.ResourceStudent, id, id, statusForErr(err))
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionUpdate, audit.ResourceStudent, updated.ID, updated.ID, http.StatusOK)
		writeJSON(w, http.StatusOK, updated)
	}
}

// teacherDayMark is one teacher's roll entry inside a day request.
type teacherDayMark struct {
	TeacherID string `json:"teacherID"`
	Status    string `json:"status"`
	Note      string `json:"note"`
}

// teacherDayRequest is the body for recording the daily teacher roll.
// An empty status means present.
type teacherDayRequest struct {
	Day   string           `json:"day"`
	Marks []teacherDayMark `json:"marks"`
}

// teacherDayRow is one teacher's mark with its display name.
type teacherDayRow struct {
	TeacherID   string `json:"TeacherID"`
	TeacherName string `json:"TeacherName"`
	Day         string `json:"Day"`
	Status      string `json:"Status"`
	Note        string `json:"Note"`
}

func parseDayParam(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().UTC(), nil
	}
	if d, err := time.Parse("2006-01-02", raw); err == nil {
		return d, nil
	}
	d, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errBadRequest
	}
	return d, nil
}

// handleRecordTeacherDay replaces the daily roll of teachers: recording
// again corrects the day. Unknown teachers fail the whole day with 404.
func handleRecordTeacherDay(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body teacherDayRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		day, err := parseDayParam(body.Day)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		subject, _ := SubjectFrom(r.Context())
		marks := make([]tattendance.DayMark, 0, len(body.Marks))
		for _, m := range body.Marks {
			id := strings.TrimSpace(m.TeacherID)
			if _, err := deps.Teachers.ByID(r.Context(), id); err != nil {
				WriteError(w, r, err)
				return
			}
			status := tattendance.StatusPresent
			if strings.TrimSpace(m.Status) != "" {
				if status, err = tattendance.Parse(m.Status); err != nil {
					WriteError(w, r, err)
					return
				}
			}
			marks = append(marks, tattendance.DayMark{
				TeacherID: id,
				Status:    status,
				Note:      strings.TrimSpace(m.Note),
			})
		}
		out, err := deps.TAttendance.RecordDay(r.Context(), tattendance.DayInput{
			Day:        day,
			Marks:      marks,
			RecordedBy: subject,
		})
		if err != nil {
			WriteError(w, r, err)
			return
		}
		for _, m := range out {
			recordAudit(deps, r, audit.ActionMark, audit.ResourceTeacherAttendance, m.TeacherID, "", http.StatusOK)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func teacherDisplayName(ctx context.Context, deps Dependencies, id string) string {
	if deps.Teachers == nil {
		return ""
	}
	if t, err := deps.Teachers.ByID(ctx, id); err == nil {
		return strings.TrimSpace(t.Names + " " + t.Surnames)
	}
	return ""
}

// handleTeacherDay returns every mark of one day with display names.
func handleTeacherDay(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		day, err := parseDayParam(r.URL.Query().Get("date"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		marks, err := deps.TAttendance.Day(r.Context(), day)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		out := make([]teacherDayRow, 0, len(marks))
		for _, m := range marks {
			out = append(out, teacherDayRow{
				TeacherID:   m.TeacherID,
				TeacherName: teacherDisplayName(r.Context(), deps, m.TeacherID),
				Day:         m.Day.Format("2006-01-02"),
				Status:      string(m.Status),
				Note:        m.Note,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// handleTeacherHistory returns one teacher's roll inside a date range
// with its summary counts, newest first.
func handleTeacherHistory(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := deps.Teachers.ByID(r.Context(), id); err != nil {
			WriteError(w, r, err)
			return
		}
		q := r.URL.Query()
		var from, to time.Time
		var err error
		if raw := strings.TrimSpace(q.Get("from")); raw != "" {
			if from, err = parseDayParam(raw); err != nil {
				WriteError(w, r, err)
				return
			}
		}
		if raw := strings.TrimSpace(q.Get("to")); raw != "" {
			if to, err = parseDayParam(raw); err != nil {
				WriteError(w, r, err)
				return
			}
		}
		marks, sum, err := deps.TAttendance.History(r.Context(), id, from, to)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"TeacherID":   id,
			"TeacherName": teacherDisplayName(r.Context(), deps, id),
			"Marks":       marks,
			"Summary":     sum,
		})
	}
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
// POST /v1/roles until the auth phase links teachers to user accounts.
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

// createUserRequest is the body for creating a login account.
type createUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	// Role is optional (e.g. "teacher"). Granting it additionally
	// requires manage-roles; admins hold every permission.
	Role string `json:"role"`
}

// userAccount is the response after creating a login account. It never
// carries the password hash (the service sanitizes, and the hash field
// is excluded from JSON anyway).
type userAccount struct {
	ID       string       `json:"ID"`
	Username string       `json:"Username"`
	Email    string       `json:"Email"`
	Active   bool         `json:"Active"`
	Roles    []roles.Role `json:"Roles"`
}

// handleCreateUser creates a login account for a teacher (or any future
// profile): username + bcrypt password, no self-registration involved.
// Only admins reach it (manage-users); the optional role grant needs
// manage-roles on top, so a lesser role can never escalate.
func handleCreateUser(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body createUserRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		u, err := deps.Users.Create(
			r.Context(),
			strings.TrimSpace(body.Username),
			strings.TrimSpace(body.Email),
			body.Password,
		)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if raw := strings.TrimSpace(body.Role); raw != "" {
			role, err := roles.Parse(raw)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			subject, _ := SubjectFrom(r.Context())
			allowed, err := deps.Auth.Can(r.Context(), subject, roles.PermissionManageRoles)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			if !allowed {
				WriteError(w, r, errForbidden)
				return
			}
			if err := deps.RolesSvc.Assign(r.Context(), u.ID, role); err != nil {
				WriteError(w, r, err)
				return
			}
		}
		rs, err := deps.Roles.Roles(r.Context(), u.ID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if rs == nil {
			rs = []roles.Role{}
		}
		writeJSON(w, http.StatusCreated, userAccount{
			ID: u.ID, Username: u.Username, Email: u.Email, Active: u.Active, Roles: rs,
		})
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

// subjectRequest is the catalog body for creating or replacing a subject.
// Grades are the school years (1-11) it is taught in; Color is #RRGGBB.
// The service assigns the ID on create and forces Active on; on replace
// Active travels in the body so retiring is an update, not a delete.
type subjectRequest struct {
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Grades      []int  `json:"grades"`
	Color       string `json:"color"`
	Active      bool   `json:"active"`
}

// handleListSubjects returns the catalog alphabetically. Teachers read it
// to name the subjects on their agenda; only admins write it.
func handleListSubjects(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := deps.Subjects.ListSubjects(r.Context())
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if list == nil {
			list = []subjects.Subject{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// handleGetSubject returns one catalog entry.
func handleGetSubject(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subj, err := deps.Subjects.SubjectByID(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, subj)
	}
}

// handleCreateSubject persists a catalog entry. The service assigns the
// ID and forces Active; any incoming ID is ignored.
func handleCreateSubject(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body subjectRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		subj, err := deps.Subjects.CreateSubject(r.Context(), subjects.Subject{
			Name: body.Name, Code: body.Code, Description: body.Description,
			Grades: body.Grades, Color: body.Color,
		})
		if err != nil {
			recordAudit(deps, r, audit.ActionCreate, audit.ResourceSubject, "", "", statusForErr(err))
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionCreate, audit.ResourceSubject, subj.ID, "", http.StatusCreated)
		writeJSON(w, http.StatusCreated, subj)
	}
}

// handleUpdateSubject replaces a catalog entry. The ID comes from the
// path; retiring (Active=false) is an update here, never a delete.
func handleUpdateSubject(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body subjectRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		subj, err := deps.Subjects.UpdateSubject(r.Context(), subjects.Subject{
			ID: chi.URLParam(r, "id"), Name: body.Name, Code: body.Code,
			Description: body.Description, Grades: body.Grades,
			Color: body.Color, Active: body.Active,
		})
		if err != nil {
			recordAudit(deps, r, audit.ActionUpdate, audit.ResourceSubject, chi.URLParam(r, "id"), "", statusForErr(err))
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionUpdate, audit.ResourceSubject, subj.ID, "", http.StatusOK)
		writeJSON(w, http.StatusOK, subj)
	}
}

// handleDeleteSubject removes a catalog entry. Blocked while assignment
// history or schedule entries reference it (400 with a stable code);
// retire with Active=false instead.
func handleDeleteSubject(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := deps.Subjects.DeleteSubject(r.Context(), id); err != nil {
			recordAudit(deps, r, audit.ActionDelete, audit.ResourceSubject, id, "", statusForErr(err))
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionDelete, audit.ResourceSubject, id, "", http.StatusNoContent)
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleSubjectAssignments returns who taught a subject and when:
// the timeline on the materia detail screen.
func handleSubjectAssignments(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := deps.Subjects.AssignmentsForSubject(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if list == nil {
			list = []subjects.Assignment{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// assignmentRequest starts a timeline entry: teacherID teaches subjectID
// from startedAt (YYYY-MM-DD) on.
type assignmentRequest struct {
	TeacherID string `json:"teacherID"`
	SubjectID string `json:"subjectID"`
	StartedAt string `json:"startedAt"`
}

// handleAssignSubject starts a teacher/subject timeline entry.
func handleAssignSubject(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body assignmentRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		startedAt, err := time.Parse("2006-01-02", strings.TrimSpace(body.StartedAt))
		if err != nil {
			WriteError(w, r, errBadRequest)
			return
		}
		a, err := deps.Subjects.Assign(r.Context(), body.TeacherID, body.SubjectID, startedAt)
		if err != nil {
			recordAudit(deps, r, audit.ActionCreate, audit.ResourceAssignment, "", "", statusForErr(err))
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionCreate, audit.ResourceAssignment, a.ID, "", http.StatusCreated)
		writeJSON(w, http.StatusCreated, a)
	}
}

// endAssignmentRequest closes an open timeline entry at endedAt
// (YYYY-MM-DD). History is kept; ending is not deleting.
type endAssignmentRequest struct {
	EndedAt string `json:"endedAt"`
}

// handleEndAssignment closes an open teacher/subject timeline entry.
func handleEndAssignment(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body endAssignmentRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		endedAt, err := time.Parse("2006-01-02", strings.TrimSpace(body.EndedAt))
		if err != nil {
			WriteError(w, r, errBadRequest)
			return
		}
		a, err := deps.Subjects.EndAssignment(r.Context(), chi.URLParam(r, "id"), endedAt)
		if err != nil {
			recordAudit(deps, r, audit.ActionUpdate, audit.ResourceAssignment, chi.URLParam(r, "id"), "", statusForErr(err))
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionUpdate, audit.ResourceAssignment, a.ID, "", http.StatusOK)
		writeJSON(w, http.StatusOK, a)
	}
}

// handleRemoveAssignment deletes a timeline entry, for corrections.
// Ending (not removing) is the way to close a finished period.
func handleRemoveAssignment(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := deps.Subjects.RemoveAssignment(r.Context(), id); err != nil {
			recordAudit(deps, r, audit.ActionDelete, audit.ResourceAssignment, id, "", statusForErr(err))
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionDelete, audit.ResourceAssignment, id, "", http.StatusNoContent)
		w.WriteHeader(http.StatusNoContent)
	}
}

// recentWindowDays bounds the lookback of the "recent" feeds: newest
// first, truncated to ?limit= (default 20, max 100). School-scale data
// makes a bounded window plus truncation cheaper than open-ended paging.
const recentWindowDays = 90

// parseRecentLimit reads ?limit=: missing means 20, over 100 clamps to
// 100, anything else is a 400.
func parseRecentLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return 20, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		WriteError(w, r, errBadRequest)
		return 0, false
	}
	if n > 100 {
		n = 100
	}
	return n, true
}

// recentWarning is one llamado row for the admin history: the stored
// warning plus display names resolved best-effort. Deleted or renamed
// records never break the list; blanks fall back to the frozen
// snapshot the warning carries.
type recentWarning struct {
	warnings.Warning
	StudentName string `json:"StudentName"`
	ClassLabel  string `json:"ClassLabel"`
	AuthorName  string `json:"AuthorName"`
}

// handleWarningsRecent returns the latest llamados, newest first.
func handleWarningsRecent(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, ok := parseRecentLimit(w, r)
		if !ok {
			return
		}
		list, err := deps.Warnings.Recent(r.Context(), time.Now().AddDate(0, 0, -recentWindowDays))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		out := make([]recentWarning, 0, len(list))
		for _, warn := range list {
			out = append(out, enrichWarning(r.Context(), deps, warn))
		}
		if len(out) > limit {
			out = out[:limit]
		}
		if out == nil {
			out = []recentWarning{}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func enrichWarning(ctx context.Context, deps Dependencies, warn warnings.Warning) recentWarning {
	e := recentWarning{Warning: warn}
	studentName := strings.TrimSpace(warn.Snapshot.Names + " " + warn.Snapshot.Surnames)
	if deps.Students != nil {
		if st, err := deps.Students.ByID(ctx, warn.StudentID); err == nil {
			studentName = strings.TrimSpace(st.Names + " " + st.Surnames)
		}
	}
	e.StudentName = studentName
	classID := strings.TrimSpace(warn.ClassID)
	if classID == "" {
		classID = strings.TrimSpace(warn.Snapshot.ClassID)
	}
	if deps.Classes != nil {
		if g, err := deps.Classes.ByID(ctx, classID); err == nil {
			e.ClassLabel = g.Label()
		}
	}
	e.AuthorName = authorName(ctx, deps, warn.TeacherID)
	return e
}

// authorName resolves who issued a warning: the stored id is the login
// subject, so a linked teacher profile wins and the username stands in
// otherwise (admins have no teacher profile).
func authorName(ctx context.Context, deps Dependencies, subject string) string {
	if deps.Teachers != nil {
		if t, err := deps.Teachers.ByID(ctx, subject); err == nil {
			return strings.TrimSpace(t.Names + " " + t.Surnames)
		}
	}
	if deps.Users == nil {
		return ""
	}
	u, err := deps.Users.ByID(ctx, subject)
	if err != nil {
		return ""
	}
	if strings.TrimSpace(u.TeacherID) != "" && deps.Teachers != nil {
		if t, err := deps.Teachers.ByID(ctx, u.TeacherID); err == nil {
			return strings.TrimSpace(t.Names + " " + t.Surnames)
		}
	}
	return u.Username
}

// recentIncident is the admin history row: faults carry no title, so the
// description head stands in (the detail view shows the full text).
type recentIncident struct {
	ID         string             `json:"ID"`
	Title      string             `json:"Title"`
	HappenedAt time.Time          `json:"HappenedAt"`
	Severity   incidents.Severity `json:"Severity"`
}

// handleIncidentsRecent returns the latest faltas, newest first.
func handleIncidentsRecent(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, ok := parseRecentLimit(w, r)
		if !ok {
			return
		}
		list, err := deps.Incidents.Recent(r.Context(), time.Now().AddDate(0, 0, -recentWindowDays))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		out := make([]recentIncident, 0, len(list))
		for _, f := range list {
			title := strings.TrimSpace(f.Description)
			if runes := []rune(title); len(runes) > 80 {
				title = string(runes[:80]) + "…"
			}
			out = append(out, recentIncident{
				ID: f.ID, Title: title, HappenedAt: f.Date, Severity: f.Severity,
			})
			if len(out) == limit {
				break
			}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// handleTeacherAssignments returns a teacher's full assignment timeline
// (open and closed): what the teacher ficha shows as "sus materias".
func handleTeacherAssignments(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := deps.Subjects.AssignmentsForTeacher(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if list == nil {
			list = []subjects.Assignment{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// scheduleRequest is the body for creating or replacing a schedule entry.
// Start/End are "HH:MM" clocks; duration and school hours are enforced
// by the service (07:00-14:00, minimum one hour).
type scheduleRequest struct {
	ClassGroupID string `json:"classGroupID"`
	TeacherID    string `json:"teacherID"`
	SubjectID    string `json:"subjectID"`
	Weekday      int    `json:"weekday"`
	Start        string `json:"start"`
	End          string `json:"end"`
}

// parseScheduleBody decodes and validates a schedule entry body. The ID
// comes from the path on replace and is empty on create (assigned).
func parseScheduleBody(w http.ResponseWriter, r *http.Request, id string) (schedules.Entry, bool) {
	var body scheduleRequest
	if !decodeJSON(w, r, &body) {
		return schedules.Entry{}, false
	}
	start, err := schedules.ParseClock(body.Start)
	if err != nil {
		WriteError(w, r, errBadRequest)
		return schedules.Entry{}, false
	}
	end, err := schedules.ParseClock(body.End)
	if err != nil {
		WriteError(w, r, errBadRequest)
		return schedules.Entry{}, false
	}
	return schedules.Entry{
		ID: id, ClassGroupID: body.ClassGroupID, TeacherID: body.TeacherID,
		SubjectID: body.SubjectID, Weekday: body.Weekday, Start: start, End: end,
	}, true
}

// handleListSchedule returns schedule entries filtered by teacher and/or
// class-group, optionally narrowed to one weekday. At least one of
// ?teacherID= / ?classGroupID= is required: no unfiltered full dump.
func handleListSchedule(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		teacherID := strings.TrimSpace(q.Get("teacherID"))
		classGroupID := strings.TrimSpace(q.Get("classGroupID"))
		if teacherID == "" && classGroupID == "" {
			WriteError(w, r, errBadRequest)
			return
		}
		var weekday *int
		if raw := strings.TrimSpace(q.Get("weekday")); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 || n > 6 {
				WriteError(w, r, errBadRequest)
				return
			}
			weekday = &n
		}
		var list []schedules.Entry
		if teacherID != "" {
			entries, err := deps.Schedules.EntriesForTeacher(r.Context(), teacherID)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			list = append(list, entries...)
		}
		if classGroupID != "" {
			entries, err := deps.Schedules.EntriesForGroup(r.Context(), classGroupID)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			if teacherID != "" {
				for _, e := range entries {
					if e.TeacherID == teacherID {
						list = append(list, e)
					}
				}
			} else {
				list = append(list, entries...)
			}
		}
		if weekday != nil {
			filtered := list[:0]
			for _, e := range list {
				if e.Weekday == *weekday {
					filtered = append(filtered, e)
				}
			}
			list = filtered
		}
		if list == nil {
			list = []schedules.Entry{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// handleCreateSchedule places one weekly slot. Overlaps (teacher or
// class-group), out-of-hours and untaught pairings fail with stable
// 400 codes the console renders inline.
func handleCreateSchedule(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entry, ok := parseScheduleBody(w, r, "")
		if !ok {
			return
		}
		created, err := deps.Schedules.Create(r.Context(), entry)
		if err != nil {
			recordAudit(deps, r, audit.ActionCreate, audit.ResourceSchedule, "", "", statusForErr(err))
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionCreate, audit.ResourceSchedule, created.ID, "", http.StatusCreated)
		writeJSON(w, http.StatusCreated, created)
	}
}

// handleUpdateSchedule replaces one weekly slot, re-checking everything.
// The ID comes from the path.
func handleUpdateSchedule(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entry, ok := parseScheduleBody(w, r, chi.URLParam(r, "id"))
		if !ok {
			return
		}
		updated, err := deps.Schedules.Update(r.Context(), entry)
		if err != nil {
			recordAudit(deps, r, audit.ActionUpdate, audit.ResourceSchedule, entry.ID, "", statusForErr(err))
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionUpdate, audit.ResourceSchedule, updated.ID, "", http.StatusOK)
		writeJSON(w, http.StatusOK, updated)
	}
}

// handleDeleteSchedule removes one weekly slot.
func handleDeleteSchedule(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := deps.Schedules.Delete(r.Context(), id); err != nil {
			recordAudit(deps, r, audit.ActionUpdate, audit.ResourceSchedule, id, "", statusForErr(err))
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionUpdate, audit.ResourceSchedule, id, "", http.StatusNoContent)
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleListAudit returns trail rows newest first: the Bitácora feed and
// the per-student file. Admin-only; the console renders 403 as
// "Acceso restringido" instead of an error.
func handleListAudit(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := 50
		if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 {
				WriteError(w, r, errBadRequest)
				return
			}
			limit = n
		}
		list, err := deps.Audit.List(r.Context(), audit.Filter{
			Limit:     limit,
			StudentID: strings.TrimSpace(q.Get("studentID")),
		})
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if list == nil {
			list = []audit.Entry{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// parseYearQuery reads a ?year= calendar year: missing, non-numeric or
// out-of-range values fail with 400 http.err_bad_request.
func parseYearQuery(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("year"))
	year, err := strconv.Atoi(raw)
	if err != nil || year < schoolyears.MinYear || year > schoolyears.MaxYear {
		WriteError(w, r, errBadRequest)
		return 0, false
	}
	return year, true
}

// createClassRequest carries grade + group number + calendar year. The
// school-year id resolves server-side, never from the body.
type createClassRequest struct {
	Year    int `json:"year"`
	Grade   int `json:"grade"`
	GroupNo int `json:"groupNo"`
}

// handleCreateClass persists a class-group (salón). Duplicates within the
// year fail with a stable 400; unknown years answer 404.
func handleCreateClass(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body createClassRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		y, err := deps.Years.ByYear(r.Context(), body.Year)
		if err != nil {
			recordAudit(deps, r, audit.ActionCreate, audit.ResourceGroup, "", "", statusForErr(err))
			WriteError(w, r, err)
			return
		}
		created, err := deps.Classes.Create(r.Context(), classes.ClassGroup{
			SchoolYearID: y.ID, Grade: body.Grade, GroupNo: body.GroupNo,
		})
		if err != nil {
			recordAudit(deps, r, audit.ActionCreate, audit.ResourceGroup, "", "", statusForErr(err))
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionCreate, audit.ResourceGroup, created.ID, "", http.StatusCreated)
		writeJSON(w, http.StatusCreated, created)
	}
}

// handleListClasses returns every class-group (salón) of the ?year=
// calendar year. The year number resolves to the id the classes service
// asks for; unknown years answer 404 like the other year lookups.
func handleListClasses(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		year, ok := parseYearQuery(w, r)
		if !ok {
			return
		}
		y, err := deps.Years.ByYear(r.Context(), year)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		groups, err := deps.Classes.ListByYear(r.Context(), y.ID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if groups == nil {
			groups = []classes.ClassGroup{}
		}
		writeJSON(w, http.StatusOK, groups)
	}
}

// sortRoster orders students alphabetically by surnames then names,
// matching the class report criterion.
func sortRoster(roster []students.Student) {
	sort.Slice(roster, func(i, j int) bool {
		a := strings.ToLower(roster[i].Surnames + " " + roster[i].Names)
		b := strings.ToLower(roster[j].Surnames + " " + roster[j].Names)
		if a == b {
			return roster[i].ID < roster[j].ID
		}
		return a < b
	})
}

// resolveRoster builds the frozen nómina of a group from its enrollments
// plus the current student profiles. Enrollments pointing at removed
// profiles are skipped, so a deleted profile never blocks the roll.
func resolveRoster(ctx context.Context, deps Dependencies, groupID string) ([]sessions.RosterEntry, error) {
	ens, err := deps.Enrollments.EnrollmentsForGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	roster := make([]sessions.RosterEntry, 0, len(ens))
	for _, e := range ens {
		st, err := deps.Students.ByID(ctx, e.StudentID)
		if err != nil {
			if errors.Is(err, students.ErrNotFound) {
				continue
			}
			return nil, err
		}
		roster = append(roster, sessions.RosterEntry{
			StudentID:  st.ID,
			Names:      st.Names,
			Surnames:   st.Surnames,
			DocumentID: st.DocumentID,
		})
	}
	return roster, nil
}

// handleClassRoster returns the group nómina: enrolled students with
// their current profiles, alphabetically.
func handleClassRoster(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "groupID")
		if _, err := deps.Classes.ByID(r.Context(), groupID); err != nil {
			WriteError(w, r, err)
			return
		}
		ens, err := deps.Enrollments.EnrollmentsForGroup(r.Context(), groupID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		roster := make([]students.Student, 0, len(ens))
		for _, e := range ens {
			st, err := deps.Students.ByID(r.Context(), e.StudentID)
			if err != nil {
				if errors.Is(err, students.ErrNotFound) {
					continue
				}
				WriteError(w, r, err)
				return
			}
			roster = append(roster, st)
		}
		sortRoster(roster)
		writeJSON(w, http.StatusOK, roster)
	}
}

// handleMySchedule returns the caller's slots for ?weekday=N (0=Sunday
// to 6=Saturday, as time.Weekday): the teacher day view. The caller is a
// login, not a profile: when the account links to a teacher, the link
// resolves; otherwise the subject itself is tried (callers whose subject
// already is a teacher id keep working).
func handleMySchedule(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimSpace(r.URL.Query().Get("weekday"))
		n, err := strconv.Atoi(raw)
		if err != nil || n < int(time.Sunday) || n > int(time.Saturday) {
			WriteError(w, r, errBadRequest)
			return
		}
		subject, _ := SubjectFrom(r.Context())
		teacherID := subject
		if u, err := deps.Users.ByID(r.Context(), subject); err == nil && u.TeacherID != "" {
			teacherID = u.TeacherID
		}
		entries, err := deps.Schedules.EntriesForTeacherOnDay(r.Context(), teacherID, time.Weekday(n))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if entries == nil {
			entries = []schedules.Entry{}
		}
		writeJSON(w, http.StatusOK, entries)
	}
}

// openSessionRequest is the body for opening an attendance call. The
// roster freezes from the group enrollments at opening time; the label,
// year and teacher resolve server-side, never from the body.
type openSessionRequest struct {
	ClassGroupID string `json:"classGroupID"`
	Period       int    `json:"period"`
	Date         string `json:"date"`
	SubjectID    string `json:"subjectID"`
}

// handleOpenSession opens an attendance call with its roster frozen as it
// is at that moment. The teacher is always the authenticated caller.
func handleOpenSession(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body openSessionRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		group, err := deps.Classes.ByID(r.Context(), strings.TrimSpace(body.ClassGroupID))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		sy, err := deps.Years.ByID(r.Context(), group.SchoolYearID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		roster, err := resolveRoster(r.Context(), deps, group.ID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		date := time.Now().UTC()
		if raw := strings.TrimSpace(body.Date); raw != "" {
			d, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				WriteError(w, r, errBadRequest)
				return
			}
			date = d
		}
		subject, _ := SubjectFrom(r.Context())
		sess, err := deps.Sessions.OpenSession(r.Context(), sessions.Session{
			ClassGroupID: group.ID,
			ClassLabel:   group.Label(),
			SchoolYear:   sy.Year,
			TeacherID:    subject,
			SubjectID:    strings.TrimSpace(body.SubjectID),
			Date:         date,
			Period:       body.Period,
			Roster:       roster,
		})
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, sess)
	}
}

// snapshotFor freezes a student profile into the snapshot stored with a
// warning, following the same shape as the demo seed.
func snapshotFor(st students.Student) warnings.StudentSnapshot {
	age := -1
	if a, ok := st.AgeAt(time.Now()); ok {
		age = a
	}
	return warnings.StudentSnapshot{
		Names: st.Names, Surnames: st.Surnames, DocumentID: st.DocumentID,
		ClassID: st.ClassID, Birthdate: st.Birthdate, Age: age,
		CaregiverName: st.Caregiver.Names, CaregiverPhone: st.Caregiver.Phone,
	}
}

// warningBatchRequest is the body for issuing one event against several
// students. Snapshots freeze from the current profiles; the teacher is
// always the authenticated caller.
type warningBatchRequest struct {
	ClassID     string   `json:"classID"`
	Gravity     string   `json:"gravity"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	HappenedAt  string   `json:"happenedAt"`
	StudentIDs  []string `json:"studentIDs"`
}

// handleIssueWarningBatch issues one event against several students: every
// warning shares a fresh group id while each student keeps its own
// history. Profiles resolve before anything persists, so an unknown
// student fails the whole batch with 404.
func handleIssueWarningBatch(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body warningBatchRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		gravity, err := warnings.Parse(body.Gravity)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		happenedAt := time.Now().UTC()
		if raw := strings.TrimSpace(body.HappenedAt); raw != "" {
			h, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				WriteError(w, r, errBadRequest)
				return
			}
			happenedAt = h
		}
		items := make([]warnings.BatchItem, 0, len(body.StudentIDs))
		for _, id := range body.StudentIDs {
			st, err := deps.Students.ByID(r.Context(), strings.TrimSpace(id))
			if err != nil {
				WriteError(w, r, err)
				return
			}
			items = append(items, warnings.BatchItem{StudentID: st.ID, Snapshot: snapshotFor(st)})
		}
		subject, _ := SubjectFrom(r.Context())
		out, err := deps.Warnings.IssueBatch(r.Context(), warnings.BatchInput{
			ClassID:     strings.TrimSpace(body.ClassID),
			TeacherID:   subject,
			HappenedAt:  happenedAt,
			Gravity:     gravity,
			Title:       strings.TrimSpace(body.Title),
			Description: strings.TrimSpace(body.Description),
			Items:       items,
		})
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

// incidentRequest is the body for reporting a fault.
type incidentRequest struct {
	StudentID   string `json:"studentID"`
	ClassID     string `json:"classID"`
	Date        string `json:"date"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

// handleReportIncident records one fault for a student.
func handleReportIncident(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body incidentRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		severity, err := incidents.Parse(body.Severity)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		date := time.Now().UTC()
		if raw := strings.TrimSpace(body.Date); raw != "" {
			d, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				WriteError(w, r, errBadRequest)
				return
			}
			date = d
		}
		fault, err := deps.Incidents.Report(
			r.Context(),
			strings.TrimSpace(body.StudentID),
			strings.TrimSpace(body.ClassID),
			date, severity,
			strings.TrimSpace(body.Description),
		)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, fault)
	}
}

// handleStudentIncidents returns one student's fault history, newest
// first: same readers as the warning history.
func handleStudentIncidents(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, _ := SubjectFrom(r.Context())
		id := chi.URLParam(r, "id")
		if !allowStudentView(r.Context(), deps.Auth, subject, id) {
			WriteError(w, r, errForbidden)
			return
		}
		list, err := deps.Incidents.ForStudent(r.Context(), id)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if list == nil {
			list = []incidents.Fault{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// handleDashboardSummary returns the admin landing aggregate for ?year=.
func handleDashboardSummary(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		year, ok := parseYearQuery(w, r)
		if !ok {
			return
		}
		summary, err := deps.Dashboard.Summary(r.Context(), year)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, summary)
	}
}
