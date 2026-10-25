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
	"github.com/google/uuid"

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
	"grade/src/core/throttle"
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
	// Throttle caps login/refresh guesses persistently. Nil means a
	// per-Router memory fallback (tests); production wires the shared
	// service from app.Services.
	Throttle *throttle.Service
	// TrustedProxies gates X-Forwarded-For for client IP. Nil trusts
	// none (direct RemoteAddr).
	TrustedProxies *throttle.Nets
	// Recovery issues single-use password resets. Nil disables the
	// endpoints (tests without recovery setup).
	Recovery *users.RecoveryService
	// RecoveryLogTokens logs raw reset tokens (development only, until
	// mail/WhatsApp delivery lands). Never enable in production.
	RecoveryLogTokens bool
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

// meResponse describes the caller for frontend navigation (roles, home)
// plus the personal profile of the login account. Account fields are
// zero-valued for dev identities (X-Subject-ID) that carry no users row.
type meResponse struct {
	Subject string       `json:"subject"`
	Roles   []roles.Role `json:"roles"`
	Home    string       `json:"home"`
	// Account profile: the login itself.
	Username  string `json:"username"`
	Email     string `json:"email"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"createdAt"`
	TeacherID string `json:"teacherID"`
	// Teacher is the linked teaching profile, when the account has one.
	Teacher *meTeacher `json:"teacher,omitempty"`
}

// meTeacher is the personal summary of the teacher linked to the login.
// It carries the full ficha: this payload only ever reaches the owner,
// so nothing here leaks to another caller.
type meTeacher struct {
	ID                string `json:"id"`
	Names             string `json:"names"`
	Surnames          string `json:"surnames"`
	DocumentID        string `json:"documentID"`
	Phone             string `json:"phone"`
	Email             string `json:"email"`
	Address           string `json:"address"`
	Birthplace        string `json:"birthplace"`
	Birthdate         string `json:"birthdate"`
	Sex               string `json:"sex"`
	MedicalConditions string `json:"medicalConditions"`
	HomeroomClassID   string `json:"homeroomClassID"`
	Active            bool   `json:"active"`
}

// meTeacherFrom maps a teacher profile onto the /me summary.
func meTeacherFrom(t teachers.Teacher) *meTeacher {
	var birth string
	if !t.Birthdate.IsZero() {
		birth = t.Birthdate.Format("2006-01-02")
	}
	return &meTeacher{
		ID: t.ID, Names: t.Names, Surnames: t.Surnames,
		DocumentID: t.DocumentID, Phone: t.Phone, Email: t.Email,
		Address: t.Address, Birthplace: t.Birthplace,
		MedicalConditions: t.MedicalConditions, HomeroomClassID: t.HomeroomClassID,
		Birthdate: birth, Sex: t.Sex, Active: t.Active,
	}
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
	case set[roles.RoleSecretary]:
		return "/admin"
	case set[roles.RoleStudent]:
		return "/estudiante"
	default:
		return "/"
	}
}

// handleMe returns the caller identity, roles, suggested home and the
// personal profile of the login account (username, email and the linked
// teacher, when any). Dev identities without a users row still get
// subject, roles and home; the account fields stay empty.
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
		resp := meResponse{Subject: subject, Roles: rs, Home: homeFor(rs)}
		if deps.Users != nil {
			u, err := deps.Users.ByID(r.Context(), subject)
			switch {
			case err == nil:
				resp.Username = u.Username
				resp.Email = u.Email
				resp.Active = u.Active
				resp.CreatedAt = u.CreatedAt.UTC().Format(time.RFC3339)
				resp.TeacherID = u.TeacherID
			case errors.Is(err, users.ErrNotFound):
				// Dev identity: no account row, nothing to enrich.
			default:
				WriteError(w, r, err)
				return
			}
			if resp.TeacherID != "" && deps.Teachers != nil {
				if t, err := deps.Teachers.ByID(r.Context(), resp.TeacherID); err == nil {
					resp.Teacher = meTeacherFrom(t)
				} else if !errors.Is(err, teachers.ErrNotFound) {
					WriteError(w, r, err)
					return
				}
			}
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// passwordChangeRequest is the body for PUT /v1/me/password. The current
// password must be repeated: the bearer alone never grants a password
// swap, so a stolen token cannot silently take over the account.
type passwordChangeRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// handleChangeMyPassword lets the signed-in account replace its own
// password. Ids and other profiles stay out of reach: nobody changes
// passwords on behalf of someone else through this route. An empty
// current password is simply a wrong one.
func handleChangeMyPassword(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, ok := SubjectFrom(r.Context())
		if !ok {
			WriteError(w, r, errUnauthorized)
			return
		}
		var body passwordChangeRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		if deps.Users == nil {
			WriteError(w, r, errInternal)
			return
		}
		if err := deps.Users.ChangePassword(r.Context(), subject, body.CurrentPassword, body.NewPassword); err != nil {
			WriteError(w, r, err)
			return
		}
		// Rotation already kills access via token_version; revoke refresh
		// rows too so stolen rotations die server-side (best effort: the
		// version check rejects them at next use either way).
		if deps.Tokens != nil {
			if err := deps.Tokens.RevokeAllForUser(r.Context(), subject); err != nil {
				slog.Warn("revoke refresh after password change failed", "error", err)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// myTeacherID resolves the caller to their own teacher profile id: the
// account link when present, the subject itself otherwise (callers whose
// subject already is a teacher id keep working). ok is false when neither
// resolves, or the linked profile no longer exists.
func myTeacherID(ctx context.Context, deps Dependencies, subject string) (string, bool) {
	if deps.Users != nil {
		if u, err := deps.Users.ByID(ctx, subject); err == nil && u.TeacherID != "" {
			if _, err := deps.Teachers.ByID(ctx, u.TeacherID); err == nil {
				return u.TeacherID, true
			}
			return "", false
		}
	}
	if _, err := deps.Teachers.ByID(ctx, subject); err == nil {
		return subject, true
	}
	return "", false
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

// errTeacherPhotoNotFound renders 404 when the teacher has no photo yet.
var errTeacherPhotoNotFound = codedError{"teachers.err_photo_not_found"}

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

// teacherUpdate is the editable part of a teacher profile. Every field
// is optional so a partial form can never silently retire a teacher or
// erase their contact data: what the body omits keeps its stored value.
type teacherUpdate struct {
	Names             *string    `json:"Names"`
	Surnames          *string    `json:"Surnames"`
	DocumentID        *string    `json:"DocumentID"`
	Phone             *string    `json:"Phone"`
	Address           *string    `json:"Address"`
	Birthplace        *string    `json:"Birthplace"`
	Birthdate         *time.Time `json:"Birthdate"`
	Email             *string    `json:"Email"`
	Sex               *string    `json:"Sex"`
	MedicalConditions *string    `json:"MedicalConditions"`
	HomeroomClassID   *string    `json:"HomeroomClassID"`
	Active            *bool      `json:"Active"`
}

// handleUpdateTeacher applies a partial update to a teacher profile.
// Retiring a teacher (Active=false) keeps the profile so old records keep
// resolving, exactly as the domain intends. Validation runs through the
// service, so an invalid field answers 400 without touching the record.
func handleUpdateTeacher(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body teacherUpdate
		if !decodeJSON(w, r, &body) {
			return
		}
		id := strings.TrimSpace(chi.URLParam(r, "id"))
		current, err := deps.Teachers.ByID(r.Context(), id)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		applyIfSet(&current.Names, body.Names)
		applyIfSet(&current.Surnames, body.Surnames)
		applyIfSet(&current.DocumentID, body.DocumentID)
		applyIfSet(&current.Phone, body.Phone)
		applyIfSet(&current.Address, body.Address)
		applyIfSet(&current.Birthplace, body.Birthplace)
		if body.Birthdate != nil {
			current.Birthdate = *body.Birthdate
		}
		applyIfSet(&current.Email, body.Email)
		applyIfSet(&current.Sex, body.Sex)
		applyIfSet(&current.MedicalConditions, body.MedicalConditions)
		applyIfSet(&current.HomeroomClassID, body.HomeroomClassID)
		if body.Active != nil {
			current.Active = *body.Active
		}
		updated, err := deps.Teachers.Update(r.Context(), current)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

// myTeacherUpdate is the self-service subset of a teacher profile: contact
// and health fields the caller may change on their own record. Identity
// (names, document), homeroom assignment and active state stay
// admin-managed through PUT /v1/teachers/{id}; unknown fields are
// rejected by the decoder, so sending them answers 400, never silent.
type myTeacherUpdate struct {
	Phone             *string    `json:"Phone"`
	Address           *string    `json:"Address"`
	Birthplace        *string    `json:"Birthplace"`
	Birthdate         *time.Time `json:"Birthdate"`
	Email             *string    `json:"Email"`
	Sex               *string    `json:"Sex"`
	MedicalConditions *string    `json:"MedicalConditions"`
}

// handleUpdateMyTeacher applies the self-service update to the caller's
// own teacher profile. Auth only, no permission: it resolves strictly to
// the caller's own id, so one teacher can never touch another profile.
func handleUpdateMyTeacher(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body myTeacherUpdate
		if !decodeJSON(w, r, &body) {
			return
		}
		subject, _ := SubjectFrom(r.Context())
		teacherID, ok := myTeacherID(r.Context(), deps, subject)
		if !ok {
			WriteError(w, r, errForbidden)
			return
		}
		current, err := deps.Teachers.ByID(r.Context(), teacherID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		applyIfSet(&current.Phone, body.Phone)
		applyIfSet(&current.Address, body.Address)
		applyIfSet(&current.Birthplace, body.Birthplace)
		if body.Birthdate != nil {
			current.Birthdate = *body.Birthdate
		}
		applyIfSet(&current.Email, body.Email)
		applyIfSet(&current.Sex, body.Sex)
		applyIfSet(&current.MedicalConditions, body.MedicalConditions)
		updated, err := deps.Teachers.Update(r.Context(), current)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

// applyIfSet copies a pointer field into target when the body provided it.
func applyIfSet(target *string, value *string) {
	if value != nil {
		*target = *value
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
		marks, sum, revisions, err := deps.TAttendance.History(r.Context(), id, from, to)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"TeacherID":   id,
			"TeacherName": teacherDisplayName(r.Context(), deps, id),
			"Marks":       marks,
			"Summary":     sum,
			"Revisions":   revisions,
		})
	}
}

// teacherRollRow is one expected teacher with its mark, if recorded.
type teacherRollRow struct {
	TeacherID   string `json:"TeacherID"`
	TeacherName string `json:"TeacherName"`
	Day         string `json:"Day"`
	Status      string `json:"Status"`
	Note        string `json:"Note"`
	Recorded    bool   `json:"Recorded"`
}

// handleTeacherRoll returns the teachers expected on a day (from the
// weekly schedule) with their recorded marks, if any. Unmarked teachers
// carry an empty status: the roll screen shows nothing pre-selected.
// On non-school days (festivos, marked no-class days) nobody is expected
// — recorded marks stay visible, and explicit rolls remain allowed.
func handleTeacherRoll(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		day, err := parseDayParam(r.URL.Query().Get("date"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		expected, err := deps.Schedules.TeachersOnWeekday(r.Context(), day.Weekday())
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if deps.Years != nil {
			if y, err := deps.Years.ByYear(r.Context(), day.Year()); err == nil && !y.IsSchoolDay(day) {
				expected = nil
			}
		}
		marks, err := deps.TAttendance.Day(r.Context(), day)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		byTeacher := make(map[string]tattendance.Mark, len(marks))
		for _, m := range marks {
			byTeacher[m.TeacherID] = m
		}
		dayLabel := day.UTC().Format("2006-01-02")
		out := make([]teacherRollRow, 0, len(expected))
		for _, id := range expected {
			name := teacherDisplayName(r.Context(), deps, id)
			if name == "" {
				continue
			}
			row := teacherRollRow{TeacherID: id, TeacherName: name, Day: dayLabel}
			if m, ok := byTeacher[id]; ok {
				row.Status = string(m.Status)
				row.Note = m.Note
				row.Recorded = true
				delete(byTeacher, id)
			}
			out = append(out, row)
		}
		// Marks for teachers no longer expected stay visible: history
		// must not silently drop a recorded day.
		for _, m := range marks {
			if _, ok := byTeacher[m.TeacherID]; !ok {
				continue
			}
			name := teacherDisplayName(r.Context(), deps, m.TeacherID)
			if name == "" {
				name = m.TeacherID
			}
			out = append(out, teacherRollRow{
				TeacherID:   m.TeacherID,
				TeacherName: name,
				Day:         dayLabel,
				Status:      string(m.Status),
				Note:        m.Note,
				Recorded:    true,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// teacherHistoryCard is one day roll for the history feed.
type teacherHistoryCard struct {
	Day     string `json:"Day"`
	Total   int    `json:"Total"`
	Present int    `json:"Present"`
	Absent  int    `json:"Absent"`
	Late    int    `json:"Late"`
}

// handleTeacherAttendanceRange returns one card per recorded day inside
// the range, newest first, with summary counts.
func handleTeacherAttendanceRange(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		marks, err := deps.TAttendance.Range(r.Context(), from, to)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		byDay := make(map[string]*teacherHistoryCard)
		var order []string
		for _, m := range marks {
			key := m.Day.Format("2006-01-02")
			card, ok := byDay[key]
			if !ok {
				card = &teacherHistoryCard{Day: key}
				byDay[key] = card
				order = append(order, key)
			}
			card.Total++
			switch m.Status {
			case tattendance.StatusPresent:
				card.Present++
			case tattendance.StatusAbsent:
				card.Absent++
			case tattendance.StatusLate:
				card.Late++
			}
		}
		out := make([]teacherHistoryCard, 0, len(order))
		for _, key := range order {
			out = append(out, *byDay[key])
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// teacherRevisionRow is one day-roll change with its author resolved.
type teacherRevisionRow struct {
	TeacherID   string `json:"TeacherID"`
	TeacherName string `json:"TeacherName"`
	Day         string `json:"Day"`
	OldStatus   string `json:"OldStatus"`
	NewStatus   string `json:"NewStatus"`
	Note        string `json:"Note"`
	Author      string `json:"Author"`
	RecordedAt  string `json:"RecordedAt"`
}

func revisionAuthor(ctx context.Context, deps Dependencies, subject string) string {
	if deps.Users != nil {
		if u, err := deps.Users.ByID(ctx, subject); err == nil && u.Username != "" {
			return u.Username
		}
	}
	if name := teacherDisplayName(ctx, deps, subject); name != "" {
		return name
	}
	if len(subject) >= 8 {
		return subject[:8]
	}
	return subject
}

// handleTeacherDayRevisions returns every change recorded on one day,
// newest first: who modified, when, and what changed.
func handleTeacherDayRevisions(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		day, err := parseDayParam(r.URL.Query().Get("date"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		revs, err := deps.TAttendance.RevisionsForDay(r.Context(), day)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		out := make([]teacherRevisionRow, 0, len(revs))
		for _, rev := range revs {
			out = append(out, teacherRevisionRow{
				TeacherID:   rev.TeacherID,
				TeacherName: teacherDisplayName(r.Context(), deps, rev.TeacherID),
				Day:         rev.Day.Format("2006-01-02"),
				OldStatus:   string(rev.OldStatus),
				NewStatus:   string(rev.NewStatus),
				Note:        rev.Note,
				Author:      revisionAuthor(r.Context(), deps, rev.RecordedBy),
				RecordedAt:  rev.RecordedAt.Format(time.RFC3339),
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// handlePutTeacherPhoto stores (or clears, when empty) a teacher photo.
func handlePutTeacherPhoto(deps Dependencies) http.HandlerFunc {
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
		if err := deps.Teachers.PutPhoto(r.Context(), chi.URLParam(r, "id"), teachers.Photo{Data: data}); err != nil {
			WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// teacherSessionRow is one roll call taken by the teacher, slimmed for
// the teacher file: date, group and absence counts.
type teacherSessionRow struct {
	ID         string `json:"ID"`
	Date       string `json:"Date"`
	ClassLabel string `json:"ClassLabel"`
	Period     int    `json:"Period"`
	Subject    string `json:"Subject"`
	Students   int    `json:"Students"`
	Absences   int    `json:"Absences"`
}

// handleTeacherSessions returns the roll calls taken by one teacher,
// newest first, capped for the file view.
func handleTeacherSessions(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := deps.Teachers.ByID(r.Context(), id); err != nil {
			WriteError(w, r, err)
			return
		}
		list, err := deps.Sessions.SessionsForTeacher(r.Context(), id)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if len(list) > 20 {
			list = list[:20]
		}
		out := make([]teacherSessionRow, 0, len(list))
		for _, sess := range list {
			row := teacherSessionRow{
				ID:         sess.ID,
				Date:       sess.Date.Format("2006-01-02"),
				ClassLabel: sess.ClassLabel,
				Period:     sess.Period,
				Students:   len(sess.Roster),
			}
			if sess.SubjectID != "" && deps.Subjects != nil {
				if subj, err := deps.Subjects.SubjectByID(r.Context(), sess.SubjectID); err == nil {
					row.Subject = subj.Name
				}
			}
			revs, err := deps.Sessions.RevisionsForSession(r.Context(), sess.ID)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			marks := make(map[string]sessions.Mark)
			for _, rev := range revs {
				marks[rev.StudentID] = rev.To
			}
			for _, m := range marks {
				if m == sessions.MarkAbsence {
					row.Absences++
				}
			}
			out = append(out, row)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// classSessionCard is one roll call for the salon history feed.
type classSessionCard struct {
	ID         string `json:"ID"`
	Date       string `json:"Date"`
	ClassLabel string `json:"ClassLabel"`
	Period     int    `json:"Period"`
	Subject    string `json:"Subject"`
	Teacher    string `json:"Teacher"`
	Students   int    `json:"Students"`
	Present    int    `json:"Present"`
	Absent     int    `json:"Absent"`
	Permission int    `json:"Permission"`
	Evasion    int    `json:"Evasion"`
	Late       int    `json:"Late"`
}

func foldSessionMarks(revs []sessions.Revision, rosterLen int) (present, absent, permission, evasion, late int) {
	marks := make(map[string]sessions.Mark)
	for _, rev := range revs {
		marks[rev.StudentID] = rev.To
	}
	for _, m := range marks {
		switch m {
		case sessions.MarkAbsence:
			absent++
		case sessions.MarkAbsencePermission:
			permission++
		case sessions.MarkEvasion:
			evasion++
		case sessions.MarkLate:
			late++
		}
	}
	present = rosterLen - absent - permission - evasion - late
	if present < 0 {
		present = 0
	}
	return present, absent, permission, evasion, late
}

func sessionCard(ctx context.Context, deps Dependencies, sess sessions.Session) (classSessionCard, error) {
	revs, err := deps.Sessions.RevisionsForSession(ctx, sess.ID)
	if err != nil {
		return classSessionCard{}, err
	}
	present, absent, permission, evasion, late := foldSessionMarks(revs, len(sess.Roster))
	card := classSessionCard{
		ID:         sess.ID,
		Date:       sess.Date.Format("2006-01-02"),
		ClassLabel: sess.ClassLabel,
		Period:     sess.Period,
		Students:   len(sess.Roster),
		Present:    present,
		Absent:     absent,
		Permission: permission,
		Evasion:    evasion,
		Late:       late,
	}
	if sess.SubjectID != "" && deps.Subjects != nil {
		if subj, err := deps.Subjects.SubjectByID(ctx, sess.SubjectID); err == nil {
			card.Subject = subj.Name
		}
	}
	if deps.Teachers != nil {
		if t, err := deps.Teachers.ByID(ctx, sess.TeacherID); err == nil {
			card.Teacher = t.FullName()
		}
	}
	return card, nil
}

// handleSessionsHistory returns the roll calls of one salon inside a
// range, newest first. Unfiltered dumps stay forbidden: a salon is
// always required.
func handleSessionsHistory(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		groupID := strings.TrimSpace(q.Get("classGroupID"))
		if groupID == "" {
			WriteError(w, r, errBadRequest)
			return
		}
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
		if to.IsZero() {
			to = time.Now().UTC()
		}
		if from.IsZero() {
			from = to.AddDate(0, 0, -30)
		}
		list, err := deps.Sessions.SessionsForGroup(r.Context(), groupID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		out := make([]classSessionCard, 0)
		for _, sess := range list {
			d := sess.Date.UTC()
			if d.Before(from) || d.After(to.AddDate(0, 0, 1).Add(-time.Nanosecond)) {
				continue
			}
			card, err := sessionCard(r.Context(), deps, sess)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			out = append(out, card)
			if len(out) >= 50 {
				break
			}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// sessionRevisionRow is one mark change with student and author resolved.
type sessionRevisionRow struct {
	Student   string `json:"Student"`
	From      string `json:"From"`
	To        string `json:"To"`
	Author    string `json:"Author"`
	ChangedAt string `json:"ChangedAt"`
	Note      string `json:"Note"`
}

// handleSessionDetail returns one roll call with its current marks and
// full revision trail for the history expand.
func handleSessionDetail(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		detail, err := deps.Sessions.SessionDetail(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		revs := make([]sessionRevisionRow, 0, len(detail.Revisions))
		for _, rev := range detail.Revisions {
			name := rev.StudentID
			for _, entry := range detail.Session.Roster {
				if entry.StudentID == rev.StudentID {
					name = strings.TrimSpace(entry.Names + " " + entry.Surnames)
					break
				}
			}
			revs = append(revs, sessionRevisionRow{
				Student:   name,
				From:      string(rev.From),
				To:        string(rev.To),
				Author:    revisionAuthor(r.Context(), deps, rev.ChangedBy),
				ChangedAt: rev.ChangedAt.Format(time.RFC3339),
				Note:      rev.Note,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"Session":   detail.Session,
			"Marks":     detail.Marks,
			"Revisions": revs,
		})
	}
}

// handleTeacherWarningsMade returns the llamados issued from the logins
// linked to one teacher, newest first with display names.
func handleTeacherWarningsMade(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, err := deps.Teachers.ByID(r.Context(), id); err != nil {
			WriteError(w, r, err)
			return
		}
		var authors []string
		if deps.Users != nil {
			if linked, err := deps.Users.ForTeacher(r.Context(), id); err == nil {
				for _, u := range linked {
					authors = append(authors, u.ID)
				}
			}
		}
		// A teacher record id doubles as author when the login reuses it.
		authors = append(authors, id)
		seen := make(map[string]warnings.Warning)
		for _, author := range authors {
			list, err := deps.Warnings.ForAuthor(r.Context(), author)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			for _, warn := range list {
				seen[warn.ID] = warn
			}
		}
		merged := make([]warnings.Warning, 0, len(seen))
		for _, warn := range seen {
			merged = append(merged, warn)
		}
		sort.Slice(merged, func(i, j int) bool {
			if merged[i].HappenedAt.Equal(merged[j].HappenedAt) {
				return merged[i].ID < merged[j].ID
			}
			return merged[i].HappenedAt.After(merged[j].HappenedAt)
		})
		if len(merged) > 50 {
			merged = merged[:50]
		}
		out := make([]recentWarning, 0, len(merged))
		for _, warn := range merged {
			out = append(out, enrichWarning(r.Context(), deps, warn))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// handleGetTeacherPhoto returns the stored teacher photo, if any.
func handleGetTeacherPhoto(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		photo, err := deps.Teachers.Photo(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if len(photo.Data) == 0 {
			WriteError(w, r, errTeacherPhotoNotFound)
			return
		}
		writeJSON(w, http.StatusOK, photoResponse{
			Data:        base64.StdEncoding.EncodeToString(photo.Data),
			ContentType: http.DetectContentType(photo.Data),
		})
	}
}

// handleGetMyTeacherPhoto returns the caller's own teacher photo. No
// extra permission: it is the caller's own image, resolved through the
// account link (or the subject when it already is a teacher id).
func handleGetMyTeacherPhoto(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, _ := SubjectFrom(r.Context())
		teacherID, ok := myTeacherID(r.Context(), deps, subject)
		if !ok {
			WriteError(w, r, errForbidden)
			return
		}
		photo, err := deps.Teachers.Photo(r.Context(), teacherID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if len(photo.Data) == 0 {
			WriteError(w, r, errTeacherPhotoNotFound)
			return
		}
		writeJSON(w, http.StatusOK, photoResponse{
			Data:        base64.StdEncoding.EncodeToString(photo.Data),
			ContentType: http.DetectContentType(photo.Data),
		})
	}
}

// handlePutMyTeacherPhoto stores (or clears, when empty) the caller's
// own teacher photo. Same self-service rule as the GET above.
func handlePutMyTeacherPhoto(deps Dependencies) http.HandlerFunc {
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
		subject, _ := SubjectFrom(r.Context())
		teacherID, ok := myTeacherID(r.Context(), deps, subject)
		if !ok {
			WriteError(w, r, errForbidden)
			return
		}
		if err := deps.Teachers.PutPhoto(r.Context(), teacherID, teachers.Photo{Data: data}); err != nil {
			WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// errAccountPhotoNotFound renders 404 when neither the linked teacher
// profile nor the account carries a stored picture.
var errAccountPhotoNotFound = codedError{"users.err_photo_not_found"}

// myPhoto returns the caller's own picture: the linked teacher photo
// when it resolves, the account photo otherwise. It is shared by the
// GET/PUT pair below. ok reports whether any storage could hold it.
func myPhoto(ctx context.Context, deps Dependencies, subject string) (teachers.Photo, bool, error) {
	if teacherID, ok := myTeacherID(ctx, deps, subject); ok {
		photo, err := deps.Teachers.Photo(ctx, teacherID)
		return photo, true, err
	}
	if deps.Users == nil {
		return teachers.Photo{}, false, nil
	}
	raw, err := deps.Users.Photo(ctx, subject)
	if err != nil {
		return teachers.Photo{}, true, err
	}
	return teachers.Photo{Data: raw.Data}, true, nil
}

// handleGetMyPhoto returns the caller's own picture. Accounts with a
// linked teacher profile keep that photo (the staff profile stays the
// single source); the rest read their own users row. Auth only, no
// permission, and no photo is an honest 404.
func handleGetMyPhoto(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, _ := SubjectFrom(r.Context())
		photo, ok, err := myPhoto(r.Context(), deps, subject)
		if !ok {
			// Dev identity without an account row: nothing owns a photo.
			WriteError(w, r, errAccountPhotoNotFound)
			return
		}
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if len(photo.Data) == 0 {
			WriteError(w, r, errAccountPhotoNotFound)
			return
		}
		writeJSON(w, http.StatusOK, photoResponse{
			Data:        base64.StdEncoding.EncodeToString(photo.Data),
			ContentType: http.DetectContentType(photo.Data),
		})
	}
}

// handlePutMyPhoto stores (or clears, when empty) the caller's own
// picture. Linked teachers write through to their profile photo; every
// other login (admins, and student logins once accounts link into
// students) stores it inside its own account row.
func handlePutMyPhoto(deps Dependencies) http.HandlerFunc {
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
		subject, _ := SubjectFrom(r.Context())
		if deps.Users == nil {
			WriteError(w, r, errInternal)
			return
		}
		if teacherID, ok := myTeacherID(r.Context(), deps, subject); ok {
			if err := deps.Teachers.PutPhoto(r.Context(), teacherID, teachers.Photo{Data: data}); err != nil {
				WriteError(w, r, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err := deps.Users.SetPhoto(r.Context(), subject, users.Photo{Data: data}); err != nil {
			WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
		out := make([]recentWarning, 0, len(list))
		for _, warn := range list {
			out = append(out, enrichWarning(r.Context(), deps, warn))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// warningUpdateRequest is the body for correcting a llamado. Empty
// fields keep the current value.
type warningUpdateRequest struct {
	Gravity     string `json:"gravity"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// handleCorrectWarning replaces the editable fields of a llamado and
// appends the change to its trail. Unchanged saves are no-ops.
func handleCorrectWarning(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body warningUpdateRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		var gravity warnings.Gravity
		if strings.TrimSpace(body.Gravity) != "" {
			var err error
			if gravity, err = warnings.Parse(body.Gravity); err != nil {
				WriteError(w, r, err)
				return
			}
		}
		subject, _ := SubjectFrom(r.Context())
		updated, err := deps.Warnings.Correct(r.Context(), id, warnings.Update{
			Gravity:     gravity,
			Title:       body.Title,
			Description: body.Description,
			ChangedBy:   subject,
		})
		if err != nil {
			recordAudit(deps, r, audit.ActionUpdate, audit.ResourceWarning, id, updated.StudentID, statusForErr(err))
			WriteError(w, r, err)
			return
		}
		recordAudit(deps, r, audit.ActionUpdate, audit.ResourceWarning, updated.ID, updated.StudentID, http.StatusOK)
		writeJSON(w, http.StatusOK, enrichWarning(r.Context(), deps, updated))
	}
}

// warningRevisionRow is one llamado correction with its author resolved.
type warningRevisionRow struct {
	WarningID  string `json:"WarningID"`
	OldGravity string `json:"OldGravity"`
	NewGravity string `json:"NewGravity"`
	OldTitle   string `json:"OldTitle"`
	NewTitle   string `json:"NewTitle"`
	OldDetail  string `json:"OldDetail"`
	NewDetail  string `json:"NewDetail"`
	Author     string `json:"Author"`
	ChangedAt  string `json:"ChangedAt"`
}

// handleWarningRevisions returns every correction of one llamado,
// newest first.
func handleWarningRevisions(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		revs, err := deps.Warnings.RevisionsForWarning(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		out := make([]warningRevisionRow, 0, len(revs))
		for _, rev := range revs {
			out = append(out, warningRevisionRow{
				WarningID:  rev.WarningID,
				OldGravity: string(rev.OldGravity),
				NewGravity: string(rev.NewGravity),
				OldTitle:   rev.OldTitle,
				NewTitle:   rev.NewTitle,
				OldDetail:  rev.OldDetail,
				NewDetail:  rev.NewDetail,
				Author:     revisionAuthor(r.Context(), deps, rev.ChangedBy),
				ChangedAt:  rev.ChangedAt.Format(time.RFC3339),
			})
		}
		writeJSON(w, http.StatusOK, out)
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
// Staff with class statistics plus the student themself (own history).
func handleStudentReport(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, _ := SubjectFrom(r.Context())
		id := strings.TrimSpace(chi.URLParam(r, "studentID"))
		if !allowStudentView(r.Context(), deps.Auth, subject, id) {
			if ok, err := deps.Auth.Can(r.Context(), subject, roles.PermissionViewClassStats); err != nil || !ok {
				WriteError(w, r, errForbidden)
				return
			}
		}
		report, err := deps.Statistics.StudentReport(r.Context(), id)
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
	// TeacherID optionally links the login to a teacher profile, so the
	// account resolves from the teacher file (lockout toggle) and the
	// teacher resolves from the login (my schedule).
	TeacherID string `json:"teacherID"`
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
		if teacherID := strings.TrimSpace(body.TeacherID); teacherID != "" {
			if deps.Teachers != nil {
				if _, err := deps.Teachers.ByID(r.Context(), teacherID); err != nil {
					WriteError(w, r, err)
					return
				}
			}
			if _, err := deps.Users.LinkTeacher(r.Context(), u.ID, teacherID); err != nil {
				WriteError(w, r, err)
				return
			}
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

// teacherAccount is the login behind a teacher profile: enough to show
// state and flip access, never the password hash.
type teacherAccount struct {
	ID       string `json:"ID"`
	Username string `json:"Username"`
	Email    string `json:"Email"`
	Active   bool   `json:"Active"`
}

// handleGetTeacherAccount resolves the login linked to a teacher profile.
// Teachers without an account answer 404 (honest empty, the admin creates
// it from the list); when several logins share the profile, the first by
// username wins and stays stable across stores.
func handleGetTeacherAccount(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(chi.URLParam(r, "id"))
		if id == "" {
			WriteError(w, r, errBadRequest)
			return
		}
		if deps.Teachers == nil || deps.Users == nil {
			WriteError(w, r, errInternal)
			return
		}
		if _, err := deps.Teachers.ByID(r.Context(), id); err != nil {
			WriteError(w, r, err)
			return
		}
		logins, err := deps.Users.ForTeacher(r.Context(), id)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if len(logins) == 0 {
			WriteError(w, r, users.ErrNotFound)
			return
		}
		sort.Slice(logins, func(i, j int) bool { return logins[i].Username < logins[j].Username })
		u := logins[0]
		writeJSON(w, http.StatusOK, teacherAccount{
			ID: u.ID, Username: u.Username, Email: u.Email, Active: u.Active,
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
	if n > 500 {
		n = 500
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
// all=1 lifts the dashboard window for admin history screens.
func handleWarningsRecent(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, ok := parseRecentLimit(w, r)
		if !ok {
			return
		}
		since := time.Now().AddDate(0, 0, -recentWindowDays)
		if strings.TrimSpace(r.URL.Query().Get("all")) == "1" {
			since = time.Time{}
		}
		list, err := deps.Warnings.Recent(r.Context(), since)
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
	// Legacy profiles store the label ("9-1") instead of the group id:
	// fall back to the live enrollment so the salon still resolves.
	if e.ClassLabel == "" && deps.Enrollments != nil {
		if enrolls, err := deps.Enrollments.EnrollmentsForStudent(ctx, warn.StudentID); err == nil {
			for _, en := range enrolls {
				if deps.Classes != nil {
					if g, err := deps.Classes.ByID(ctx, en.ClassGroupID); err == nil {
						e.ClassLabel = g.Label()
						break
					}
				}
			}
		}
	}
	// Last resort: the id itself is already a human label. Short opaque
	// strings display verbatim so legacy rows match the student file,
	// which shows the raw profile value.
	if e.ClassLabel == "" {
		if raw := strings.TrimSpace(warn.ClassID); raw != "" && len(raw) <= 20 {
			if _, err := uuid.Parse(raw); err != nil {
				e.ClassLabel = raw
			}
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

// ensureAssignment guarantees an open teacher-subject pairing before a
// schedule write: assigning happens automatically when a class is placed
// on the schedule, never by hand. Assigning twice is a no-op thanks to
// the open-pair check (Assign itself would fail duplicate).
func ensureAssignment(ctx context.Context, deps Dependencies, teacherID, subjectID string) error {
	// Stacks without a catalog (checker bypass) skip: production always
	// wires Subjects.
	if deps.Subjects == nil {
		return nil
	}
	timeline, err := deps.Subjects.AssignmentsForTeacher(ctx, teacherID)
	if err != nil {
		return err
	}
	for _, a := range timeline {
		if a.SubjectID == subjectID && a.IsOpen() {
			return nil
		}
	}
	_, err = deps.Subjects.Assign(ctx, teacherID, subjectID, time.Now().UTC())
	return err
}

// handleCreateSchedule places one weekly slot. Overlaps (teacher or
// class-group) and out-of-hours fail with stable 400 codes the console
// renders inline; the teacher-subject pairing is assigned automatically.
func handleCreateSchedule(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entry, ok := parseScheduleBody(w, r, "")
		if !ok {
			return
		}
		if err := ensureAssignment(r.Context(), deps, entry.TeacherID, entry.SubjectID); err != nil {
			WriteError(w, r, err)
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
		if err := ensureAssignment(r.Context(), deps, entry.TeacherID, entry.SubjectID); err != nil {
			WriteError(w, r, err)
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
