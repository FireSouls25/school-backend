package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"grade/src/core/classes"
	"grade/src/core/enrollments"
	"grade/src/core/incidents"
	"grade/src/core/roles"
	"grade/src/core/schedules"
	"grade/src/core/schoolyears"
	"grade/src/core/sessions"
	"grade/src/core/statistics"
	"grade/src/core/students"
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
	Auth        roles.Authorizer
	Roles       roles.RoleGetter
	RolesSvc    *roles.Service
	Students    *students.Service
	Teachers    *teachers.Service
	Users       *users.Service
	Warnings    *warnings.Service
	Sessions    *sessions.Service
	Statistics  *statistics.Service
	Dashboard   *statistics.DashboardService
	Classes     *classes.Service
	Years       *schoolyears.Service
	Enrollments *enrollments.Service
	Schedules   *schedules.Service
	Incidents   *incidents.Service
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
// to 6=Saturday, as time.Weekday): the teacher day view.
func handleMySchedule(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimSpace(r.URL.Query().Get("weekday"))
		n, err := strconv.Atoi(raw)
		if err != nil || n < int(time.Sunday) || n > int(time.Saturday) {
			WriteError(w, r, errBadRequest)
			return
		}
		subject, _ := SubjectFrom(r.Context())
		entries, err := deps.Schedules.EntriesForTeacherOnDay(r.Context(), subject, time.Weekday(n))
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
