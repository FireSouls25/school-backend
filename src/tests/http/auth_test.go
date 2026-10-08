package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

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
	httpapi "grade/src/platform/http"
)

// Fixture identities for pure subjects. Student record ids come from the
// services (which always assign fresh UUIDs); the student *subjects* below
// are set at setup time. View-own-history only opens the caller's own id,
// so each student subject doubles as its record id.
const (
	adminSubject   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	teacherSubject = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	unknownSubject = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	groupID        = "ffffffff-ffff-4fff-8fff-ffffffffffff"
)

var (
	studentSubject string
	otherStudent   string
	sessionRouteID string
	realGroupID    string
)

func validStudent(id string) students.Student {
	return students.Student{
		ID: id, Names: "Ana", Surnames: "Gómez", ClassID: "9-1",
		DocumentID: "1234567890",
		Caregiver:  students.Guardian{Names: "María Gómez"},
	}
}

// allowAllChecker is a schedules.Checker stub: every pairing teaches.
// The day view never consults it; only entry creation does.
type allowAllChecker struct{}

func (allowAllChecker) Teaches(context.Context, string, string) (bool, error) {
	return true, nil
}

// testSessions adapts the sessions service to statistics.SessionSource,
// mirroring the composition root.
type testSessions struct {
	svc *sessions.Service
}

func (t testSessions) GroupSessions(ctx context.Context, gid string) ([]statistics.SessionView, error) {
	list, err := t.svc.SessionsForGroup(ctx, gid)
	if err != nil {
		return nil, err
	}
	return t.withMarks(ctx, list)
}

func (t testSessions) StudentSessions(ctx context.Context, id string) ([]statistics.SessionView, error) {
	list, err := t.svc.SessionsForStudent(ctx, id)
	if err != nil {
		return nil, err
	}
	return t.withMarks(ctx, list)
}

func (t testSessions) withMarks(ctx context.Context, list []sessions.Session) ([]statistics.SessionView, error) {
	out := make([]statistics.SessionView, 0, len(list))
	for _, sess := range list {
		detail, err := t.svc.SessionDetail(ctx, sess.ID)
		if err != nil {
			return nil, err
		}
		view := statistics.SessionView{
			ID:           sess.ID,
			ClassGroupID: sess.ClassGroupID,
			ClassLabel:   sess.ClassLabel,
			SchoolYear:   sess.SchoolYear,
			Date:         sess.Date,
			Period:       sess.Period,
			Marks:        make(map[string]string, len(detail.Marks)),
		}
		for _, e := range sess.Roster {
			view.Roster = append(view.Roster, statistics.RosterEntryView{
				StudentID: e.StudentID, Names: e.Names, Surnames: e.Surnames,
			})
		}
		for id, mark := range detail.Marks {
			view.Marks[id] = mark.String()
		}
		out = append(out, view)
	}
	return out, nil
}

// testWarnings adapts the warnings service to statistics.WarningSource,
// mirroring the composition root.
type testWarnings struct {
	svc *warnings.Service
}

func (t testWarnings) StudentWarnings(ctx context.Context, id string) ([]statistics.WarningView, error) {
	list, err := t.svc.ForStudent(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]statistics.WarningView, 0, len(list))
	for _, w := range list {
		out = append(out, statistics.WarningView{Gravity: w.Gravity.String(), Date: w.HappenedAt})
	}
	return out, nil
}

// testFaults adapts the incidents service to statistics.FaultSource,
// mirroring the composition root.
type testFaults struct {
	svc *incidents.Service
}

func (t testFaults) StudentFaults(ctx context.Context, id string) ([]statistics.FaultView, error) {
	list, err := t.svc.ForStudent(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]statistics.FaultView, 0, len(list))
	for _, f := range list {
		out = append(out, statistics.FaultView{Severity: f.Severity.String(), Date: f.Date})
	}
	return out, nil
}

// testDash adapts the domain services to statistics.DashboardSource,
// mirroring the composition root.
type testDash struct {
	teachers    *teachers.Service
	years       *schoolyears.Service
	classes     *classes.Service
	enrollments *enrollments.Service
	warnings    *warnings.Service
}

func (d testDash) TeacherTotal(ctx context.Context) (int, error) {
	list, err := d.teachers.List(ctx)
	if err != nil {
		return 0, err
	}
	return len(list), nil
}

func (d testDash) GroupsOfYear(ctx context.Context, year int) ([]string, error) {
	y, err := d.years.ByYear(ctx, year)
	if err != nil {
		return nil, err
	}
	groups, err := d.classes.ListByYear(ctx, y.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.ID)
	}
	return ids, nil
}

func (d testDash) EnrolledIDs(ctx context.Context, gid string) ([]string, error) {
	ens, err := d.enrollments.EnrollmentsForGroup(ctx, gid)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(ens))
	for _, e := range ens {
		ids = append(ids, e.StudentID)
	}
	return ids, nil
}

func (d testDash) RecentWarnings(ctx context.Context, since time.Time) ([]statistics.DashboardWarning, error) {
	list, err := d.warnings.Recent(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]statistics.DashboardWarning, 0, len(list))
	for _, w := range list {
		out = append(out, statistics.DashboardWarning{
			ID: w.ID, StudentID: w.StudentID, ClassID: w.ClassID,
			TeacherID: w.TeacherID, Gravity: w.Gravity.String(),
			Title: w.Title, Date: w.HappenedAt,
		})
	}
	return out, nil
}

func testStack(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()

	roleSvc := roles.NewService(roles.NewMemoryStore())
	studentsSvc := students.NewService(students.NewMemoryStore())
	teachersSvc := teachers.NewService(teachers.NewMemoryStore())
	warningsSvc := warnings.NewService(warnings.NewMemoryStore())
	sessionsSvc := sessions.NewService(sessions.NewMemoryStore())
	yearsSvc := schoolyears.NewService(schoolyears.NewMemoryStore())
	classesSvc := classes.NewService(classes.NewMemoryStore())
	enrollmentsSvc := enrollments.NewService(enrollments.NewMemoryStore())
	schedulesSvc := schedules.NewService(schedules.NewMemoryStore(), allowAllChecker{})
	incidentsSvc := incidents.NewService(incidents.NewMemoryStore())
	sessViews := testSessions{svc: sessionsSvc}
	statsSvc := statistics.NewService(sessViews, testWarnings{warningsSvc}, testFaults{incidentsSvc})
	dashSvc := statistics.NewDashboardService(sessViews, testWarnings{warningsSvc}, testFaults{incidentsSvc}, testDash{
		teachers:    teachersSvc,
		years:       yearsSvc,
		classes:     classesSvc,
		enrollments: enrollmentsSvc,
		warnings:    warningsSvc,
	})

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	must(roleSvc.Assign(ctx, teacherSubject, roles.RoleTeacher))

	year, err := yearsSvc.Create(ctx, schoolyears.SchoolYear{
		Year: 2026, Periods: 3,
		StartDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC),
	})
	must(err)
	group, err := classesSvc.Create(ctx, classes.ClassGroup{
		SchoolYearID: year.ID, Grade: 9, GroupNo: 1,
	})
	must(err)
	realGroupID = group.ID

	// Services assign fresh ids: capture them so each student subject is
	// also its own record id (what view-own-history checks).
	stA, err := studentsSvc.Create(ctx, validStudent(""))
	if err != nil {
		t.Fatalf("create student: %v", err)
	}
	studentSubject = stA.ID
	must(roleSvc.Assign(ctx, studentSubject, roles.RoleStudent))
	stB, err := studentsSvc.Create(ctx, validStudent(""))
	if err != nil {
		t.Fatalf("create other student: %v", err)
	}
	otherStudent = stB.ID
	must(roleSvc.Assign(ctx, otherStudent, roles.RoleStudent))

	// Both students belong to the real group, so roster, open-session
	// and dashboard fixtures fold them.
	for _, st := range []students.Student{stA, stB} {
		_, err := enrollmentsSvc.Enroll(ctx, enrollments.Enrollment{
			StudentID: st.ID, ClassGroupID: realGroupID,
		})
		must(err)
	}

	// One Monday slot for the teacher day view.
	_, err = schedulesSvc.Create(ctx, schedules.Entry{
		ClassGroupID: realGroupID,
		TeacherID:    teacherSubject,
		SubjectID:    "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		Weekday:      int(time.Monday),
		Start:        schedules.MustClock("07:00"),
		End:          schedules.MustClock("09:00"),
	})
	must(err)
	sess, err := sessionsSvc.OpenSession(ctx, sessions.Session{
		ClassGroupID: realGroupID,
		ClassLabel:   "9-1",
		SchoolYear:   2026,
		TeacherID:    teacherSubject,
		Date:         time.Now().UTC(),
		Period:       2,
		Roster: []sessions.RosterEntry{
			{StudentID: studentSubject, Names: "Ana", Surnames: "Gómez", DocumentID: "1234567890"},
		},
	})
	must(err)
	sessionRouteID = sess.ID

	return httpapi.Router(httpapi.Dependencies{
		Auth:        roleSvc,
		Roles:       roleSvc,
		RolesSvc:    roleSvc,
		Students:    studentsSvc,
		Teachers:    teachersSvc,
		Users:       users.NewServiceWithCost(users.NewMemoryStore(), bcrypt.MinCost),
		Warnings:    warningsSvc,
		Sessions:    sessionsSvc,
		Statistics:  statsSvc,
		Dashboard:   dashSvc,
		Classes:     classesSvc,
		Years:       yearsSvc,
		Enrollments: enrollmentsSvc,
		Schedules:   schedulesSvc,
		Incidents:   incidentsSvc,
	}, newI18n(t))
}

func doRequest(t *testing.T, h http.Handler, method, target, subject, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if subject != "" {
		req.Header.Set(httpapi.SubjectHeader, subject)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Error.Message == "" {
		t.Errorf("empty localized message for code %q", body.Error.Code)
	}
	return body.Error.Code
}

func TestMissingOrBadSubjectIs401(t *testing.T) {
	h := testStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/me", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no subject status = %d, want 401", rec.Code)
	}
	if code := errorCode(t, rec); code != "http.err_unauthorized" {
		t.Errorf("code = %q, want http.err_unauthorized", code)
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/me", "not-a-uuid", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("bad subject status = %d, want 401", rec.Code)
	}
}

func TestMeReportsRolesAndHome(t *testing.T) {
	h := testStack(t)

	for _, tc := range []struct {
		subject string
		role    roles.Role
		home    string
	}{
		{adminSubject, roles.RoleAdmin, "/admin"},
		{teacherSubject, roles.RoleTeacher, "/docente"},
		{studentSubject, roles.RoleStudent, "/estudiante"},
	} {
		rec := doRequest(t, h, http.MethodGet, "/v1/me", tc.subject, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", tc.subject, rec.Code)
		}
		var body struct {
			Subject string       `json:"subject"`
			Roles   []roles.Role `json:"roles"`
			Home    string       `json:"home"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode me: %v", err)
		}
		if body.Subject != tc.subject || body.Home != tc.home || len(body.Roles) != 1 || body.Roles[0] != tc.role {
			t.Errorf("me = %+v, want role %q home %q", body, tc.role, tc.home)
		}
	}
}

func TestTeacherBlockedFromAdminStudentReport(t *testing.T) {
	h := testStack(t)
	target := "/v1/statistics/student/" + studentSubject

	rec := doRequest(t, h, http.MethodGet, target, teacherSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("teacher status = %d, want 403", rec.Code)
	}
	if code := errorCode(t, rec); code != "http.err_forbidden" {
		t.Errorf("code = %q, want http.err_forbidden", code)
	}
	if strings.Contains(rec.Body.String(), "Ana") {
		t.Errorf("denied response leaks student data: %s", rec.Body.String())
	}

	rec = doRequest(t, h, http.MethodGet, target, adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Errorf("admin status = %d, want 200", rec.Code)
	}
}

func TestClassStatsNeedClassPermission(t *testing.T) {
	h := testStack(t)
	target := "/v1/statistics/class/" + groupID

	rec := doRequest(t, h, http.MethodGet, target, teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Errorf("teacher status = %d, want 200", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, target, adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Errorf("admin status = %d, want 200", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, target, studentSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("student status = %d, want 403", rec.Code)
	}
}

func TestStudentReads(t *testing.T) {
	h := testStack(t)

	// Own profile: allowed.
	rec := doRequest(t, h, http.MethodGet, "/v1/students/"+studentSubject, studentSubject, "")
	if rec.Code != http.StatusOK {
		t.Errorf("own profile status = %d, want 200", rec.Code)
	}
	// Another student's profile: denied.
	rec = doRequest(t, h, http.MethodGet, "/v1/students/"+otherStudent, studentSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("other profile status = %d, want 403", rec.Code)
	}
	// Teacher reads anyone.
	rec = doRequest(t, h, http.MethodGet, "/v1/students/"+otherStudent, teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Errorf("teacher read status = %d, want 200", rec.Code)
	}
	// Subject without roles reads nothing.
	rec = doRequest(t, h, http.MethodGet, "/v1/students/"+studentSubject, unknownSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("unknown subject status = %d, want 403", rec.Code)
	}
	// Own warnings: allowed.
	rec = doRequest(t, h, http.MethodGet, "/v1/students/"+studentSubject+"/warnings", studentSubject, "")
	if rec.Code != http.StatusOK {
		t.Errorf("own warnings status = %d, want 200", rec.Code)
	}
}

func TestRecordMarkGuarded(t *testing.T) {
	h := testStack(t)
	target := "/v1/sessions/" + sessionRouteID + "/marks"
	payload := `{"studentID":"` + studentSubject + `","mark":"atraso","note":"llegó 8:05"}`

	rec := doRequest(t, h, http.MethodPost, target, teacherSubject, payload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("teacher mark status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var rev struct {
		Number int    `json:"Number"`
		To     string `json:"To"`
	}
	// Domain structs serialize with Go field names.
	if err := json.Unmarshal(rec.Body.Bytes(), &rev); err != nil {
		t.Fatalf("decode revision: %v", err)
	}
	if rev.Number != 1 || rev.To != "late" {
		t.Errorf("revision = %+v, want number 1 to late", rev)
	}

	rec = doRequest(t, h, http.MethodPost, target, studentSubject, payload)
	if rec.Code != http.StatusForbidden {
		t.Errorf("student mark status = %d, want 403", rec.Code)
	}

	rec = doRequest(t, h, http.MethodPost, target, "", payload)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous mark status = %d, want 401", rec.Code)
	}

	rec = doRequest(t, h, http.MethodPost, target, teacherSubject, `{"studentID":"x","mark":"atraso"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad student status = %d, want 400", rec.Code)
	}

	rec = doRequest(t, h, http.MethodPost, target, teacherSubject, `{"studentID":"`+studentSubject+`","mark":"sick"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad mark status = %d, want 400", rec.Code)
	}

	rec = doRequest(t, h, http.MethodPost, target, teacherSubject, `{"studentID":"`+studentSubject+`","mark":"atraso","hacker":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field status = %d, want 400", rec.Code)
	}
}

func TestAdminCreatesStudent(t *testing.T) {
	h := testStack(t)

	body := `{"Names":"Pedro","Surnames":"López","ClassID":"9-1",` +
		`"DocumentID":"1020304050",` +
		`"Caregiver":{"Names":"María López"}}`
	rec := doRequest(t, h, http.MethodPost, "/v1/students", adminSubject, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var st struct {
		ID     string `json:"ID"`
		Status string `json:"Status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode student: %v", err)
	}
	if st.ID == "" || st.Status != "active" {
		t.Errorf("created = %+v, want id and active status", st)
	}

	// Teacher cannot create students.
	rec = doRequest(t, h, http.MethodPost, "/v1/students", teacherSubject, body)
	if rec.Code != http.StatusForbidden {
		t.Errorf("teacher create status = %d, want 403", rec.Code)
	}

	// Validation errors surface as 400 with stable codes.
	rec = doRequest(t, h, http.MethodPost, "/v1/students", adminSubject, `{"Names":"Pedro"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid create status = %d, want 400", rec.Code)
	}
	if code := errorCode(t, rec); code == "" || code == "http.err_internal" {
		t.Errorf("code = %q, want a stable domain code", code)
	}
}

func TestAdminCreatesTeacherAndGrantsRole(t *testing.T) {
	h := testStack(t)

	body := `{"Names":"Carlos","Surnames":"Mendoza","DocumentID":"79888777"}`
	rec := doRequest(t, h, http.MethodPost, "/v1/teachers", adminSubject, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var tc struct {
		ID     string `json:"ID"`
		Active bool   `json:"Active"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tc); err != nil {
		t.Fatalf("decode teacher: %v", err)
	}
	if tc.ID == "" || !tc.Active {
		t.Errorf("created = %+v, want id and active", tc)
	}

	// Teacher cannot create teachers.
	rec = doRequest(t, h, http.MethodPost, "/v1/teachers", teacherSubject, body)
	if rec.Code != http.StatusForbidden {
		t.Errorf("teacher create status = %d, want 403", rec.Code)
	}

	// Grant the teacher role to a fresh subject id.
	newSubject := "12345678-1234-4234-8234-123456789012"
	rec = doRequest(t, h, http.MethodPost, "/v1/roles",
		adminSubject, `{"subjectID":"`+newSubject+`","role":"teacher"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("grant status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	// Unknown roles fail with a stable code, not a 500.
	rec = doRequest(t, h, http.MethodPost, "/v1/roles",
		adminSubject, `{"subjectID":"`+newSubject+`","role":"principal"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad role status = %d, want 400", rec.Code)
	}
	if code := errorCode(t, rec); code != "roles.err_unknown_role" {
		t.Errorf("code = %q, want roles.err_unknown_role", code)
	}

	// Teacher cannot grant roles.
	rec = doRequest(t, h, http.MethodPost, "/v1/roles",
		teacherSubject, `{"subjectID":"`+newSubject+`","role":"teacher"}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("teacher grant status = %d, want 403", rec.Code)
	}
}

func TestAdminListsAndReadsTeachers(t *testing.T) {
	h := testStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/teachers", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin list status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/teachers", teacherSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("teacher list status = %d, want 403", rec.Code)
	}

	created := doRequest(t, h, http.MethodPost, "/v1/teachers", adminSubject,
		`{"Names":"Ana","Surnames":"Ríos","DocumentID":"51999111"}`)
	var tc struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &tc); err != nil || tc.ID == "" {
		t.Fatalf("setup create: %v %s", err, created.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/"+tc.ID, adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Errorf("admin read status = %d, want 200", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/00000000-0000-4000-8000-000000000000", adminSubject, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown teacher status = %d, want 404", rec.Code)
	}
}

func TestAdminCreatesUserAccount(t *testing.T) {
	h := testStack(t)

	// Admin creates a login account with the teacher role in one call.
	rec := doRequest(t, h, http.MethodPost, "/v1/users", adminSubject,
		`{"username":"laura.torres","email":"laura.torres@observador.edu.co","password":"docente123*","role":"teacher"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var acct struct {
		ID       string       `json:"ID"`
		Username string       `json:"Username"`
		Email    string       `json:"Email"`
		Active   bool         `json:"Active"`
		Roles    []roles.Role `json:"Roles"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &acct); err != nil {
		t.Fatalf("decode account: %v", err)
	}
	if acct.ID == "" || acct.Username != "laura.torres" || !acct.Active {
		t.Errorf("account = %+v, want id, username and active", acct)
	}
	if len(acct.Roles) != 1 || acct.Roles[0] != roles.RoleTeacher {
		t.Errorf("roles = %+v, want [teacher]", acct.Roles)
	}
	if strings.Contains(rec.Body.String(), "hash") || strings.Contains(rec.Body.String(), "$2a$") {
		t.Errorf("response leaks password hash: %s", rec.Body.String())
	}

	// Teacher cannot create accounts.
	rec = doRequest(t, h, http.MethodPost, "/v1/users", teacherSubject,
		`{"username":"otro.docente","password":"docente123*"}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("teacher create status = %d, want 403", rec.Code)
	}

	// Duplicates fail with a stable code, not a 500.
	rec = doRequest(t, h, http.MethodPost, "/v1/users", adminSubject,
		`{"username":"LAURA.TORRES","password":"docente123*"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate status = %d, want 400", rec.Code)
	}
	if code := errorCode(t, rec); code != "users.err_duplicate_username" {
		t.Errorf("code = %q, want users.err_duplicate_username", code)
	}

	// Weak passwords are rejected before anything is stored.
	rec = doRequest(t, h, http.MethodPost, "/v1/users", adminSubject,
		`{"username":"debil.cuenta","password":"corta"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("weak password status = %d, want 400", rec.Code)
	}
	if code := errorCode(t, rec); code != "users.err_weak_password" {
		t.Errorf("code = %q, want users.err_weak_password", code)
	}

	// Unknown roles fail with a stable code.
	rec = doRequest(t, h, http.MethodPost, "/v1/users", adminSubject,
		`{"username":"rara.cuenta","password":"docente123*","role":"principal"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad role status = %d, want 400", rec.Code)
	}
	if code := errorCode(t, rec); code != "roles.err_unknown_role" {
		t.Errorf("code = %q, want roles.err_unknown_role", code)
	}
}

func TestListClassesByYear(t *testing.T) {
	h := testStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/classes?year=2026", teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("teacher status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var groups []struct {
		ID      string `json:"ID"`
		Grade   int    `json:"Grade"`
		GroupNo int    `json:"GroupNo"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &groups); err != nil {
		t.Fatalf("decode groups: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != realGroupID || groups[0].Grade != 9 {
		t.Errorf("groups = %+v, want the 9-1 group", groups)
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/classes?year=2026", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Errorf("admin status = %d, want 200", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/classes?year=2026", studentSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("student status = %d, want 403", rec.Code)
	}
	for _, target := range []string{"/v1/classes", "/v1/classes?year=abc", "/v1/classes?year=1999"} {
		rec = doRequest(t, h, http.MethodGet, target, teacherSubject, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400", target, rec.Code)
		}
		if code := errorCode(t, rec); code != "http.err_bad_request" {
			t.Errorf("GET %s code = %q, want http.err_bad_request", target, code)
		}
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/classes?year=2030", teacherSubject, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown year status = %d, want 404", rec.Code)
	}
}

func TestClassRosterAlphabetical(t *testing.T) {
	h := testStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/classes/"+realGroupID+"/roster", teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("teacher status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var roster []struct {
		ID       string `json:"ID"`
		Surnames string `json:"Surnames"`
		Names    string `json:"Names"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &roster); err != nil {
		t.Fatalf("decode roster: %v", err)
	}
	if len(roster) != 2 {
		t.Fatalf("len(roster) = %d, want 2", len(roster))
	}
	for i := 1; i < len(roster); i++ {
		prev := roster[i-1].Surnames + " " + roster[i-1].Names
		cur := roster[i].Surnames + " " + roster[i].Names
		if prev > cur {
			t.Errorf("roster out of order: %q before %q", prev, cur)
		}
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/classes/"+realGroupID+"/roster", studentSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("student status = %d, want 403", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/classes/not-a-uuid/roster", teacherSubject, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad group status = %d, want 400", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/classes/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/roster", teacherSubject, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown group status = %d, want 404", rec.Code)
	}
}

func TestMyScheduleDayView(t *testing.T) {
	h := testStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/teachers/me/schedule?weekday=1", teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("teacher status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var entries []struct {
		TeacherID string `json:"TeacherID"`
		Weekday   int    `json:"Weekday"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode schedule: %v", err)
	}
	if len(entries) != 1 || entries[0].TeacherID != teacherSubject || entries[0].Weekday != 1 {
		t.Errorf("entries = %+v, want the Monday slot", entries)
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/me/schedule?weekday=3", teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("empty day status = %d, want 200", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty day body = %s, want []", rec.Body.String())
	}
	for _, target := range []string{"/v1/teachers/me/schedule", "/v1/teachers/me/schedule?weekday=7", "/v1/teachers/me/schedule?weekday=lunes"} {
		rec = doRequest(t, h, http.MethodGet, target, teacherSubject, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400", target, rec.Code)
		}
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/me/schedule?weekday=1", studentSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("student status = %d, want 403", rec.Code)
	}
}

func TestOpenSessionFreezesRoster(t *testing.T) {
	h := testStack(t)

	rec := doRequest(t, h, http.MethodPost, "/v1/sessions/open", teacherSubject,
		`{"classGroupID":"`+realGroupID+`","period":1}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("teacher status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var sess struct {
		TeacherID  string `json:"TeacherID"`
		ClassLabel string `json:"ClassLabel"`
		SchoolYear int    `json:"SchoolYear"`
		Roster     []struct {
			StudentID string `json:"StudentID"`
		} `json:"Roster"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if sess.TeacherID != teacherSubject {
		t.Errorf("TeacherID = %q, want the caller", sess.TeacherID)
	}
	if sess.ClassLabel != "9-1" || sess.SchoolYear != 2026 {
		t.Errorf("session = %+v, want frozen 9-1/2026", sess)
	}
	if len(sess.Roster) != 2 {
		t.Errorf("len(roster) = %d, want both enrolled students", len(sess.Roster))
	}

	rec = doRequest(t, h, http.MethodPost, "/v1/sessions/open", teacherSubject,
		`{"classGroupID":"`+realGroupID+`","period":1,"hacker":true}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field status = %d, want 400", rec.Code)
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/sessions/open", teacherSubject,
		`{"period":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing group status = %d, want 400", rec.Code)
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/sessions/open", teacherSubject,
		`{"classGroupID":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","period":1}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown group status = %d, want 404", rec.Code)
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/sessions/open", studentSubject,
		`{"classGroupID":"`+realGroupID+`","period":1}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("student status = %d, want 403", rec.Code)
	}
}

func TestWarningBatchGroupsEvent(t *testing.T) {
	h := testStack(t)

	payload := `{"classID":"9-1","gravity":"leve","title":"Bulla en clase",` +
		`"description":"Hablaban durante la explicación.",` +
		`"studentIDs":["` + studentSubject + `","` + otherStudent + `"]}`
	rec := doRequest(t, h, http.MethodPost, "/v1/warnings/batch", teacherSubject, payload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("teacher status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var out []struct {
		ID        string `json:"ID"`
		StudentID string `json:"StudentID"`
		TeacherID string `json:"TeacherID"`
		GroupID   string `json:"GroupID"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode batch: %v", err)
	}
	if len(out) != 2 || out[0].GroupID == "" || out[0].GroupID != out[1].GroupID {
		t.Fatalf("batch = %+v, want two warnings sharing a group", out)
	}
	if out[0].TeacherID != teacherSubject {
		t.Errorf("TeacherID = %q, want the caller", out[0].TeacherID)
	}

	// Each student keeps its own history including the batch warning.
	rec = doRequest(t, h, http.MethodGet, "/v1/students/"+studentSubject+"/warnings", teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("history status = %d, want 200", rec.Code)
	}
	var hist []struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &hist); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(hist) != 1 {
		t.Errorf("len(history) = %d, want 1", len(hist))
	}

	rec = doRequest(t, h, http.MethodPost, "/v1/warnings/batch", teacherSubject,
		`{"classID":"9-1","gravity":"leve","title":"X","description":"Y","studentIDs":[]}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty batch status = %d, want 400", rec.Code)
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/warnings/batch", teacherSubject,
		`{"classID":"9-1","gravity":"inexistente","title":"X","description":"Y","studentIDs":["`+studentSubject+`"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad gravity status = %d, want 400", rec.Code)
	}
	if code := errorCode(t, rec); code != "warnings.err_unknown_gravity" {
		t.Errorf("code = %q, want warnings.err_unknown_gravity", code)
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/warnings/batch", studentSubject, payload)
	if rec.Code != http.StatusForbidden {
		t.Errorf("student status = %d, want 403", rec.Code)
	}
}

func TestIncidentReportAndStudentHistory(t *testing.T) {
	h := testStack(t)

	payload := `{"studentID":"` + studentSubject + `","classID":"9-1",` +
		`"severity":"leve","description":"Uso del celular en clase."}`
	rec := doRequest(t, h, http.MethodPost, "/v1/incidents", teacherSubject, payload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("teacher report status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var fault struct {
		StudentID string `json:"StudentID"`
		Severity  string `json:"Severity"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &fault); err != nil {
		t.Fatalf("decode fault: %v", err)
	}
	if fault.StudentID != studentSubject || fault.Severity != "minor" {
		t.Errorf("fault = %+v, want the reported minor fault", fault)
	}

	// Teacher reads anyone; the student reads only their own history.
	rec = doRequest(t, h, http.MethodGet, "/v1/students/"+studentSubject+"/incidents", teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("teacher read status = %d, want 200", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/students/"+studentSubject+"/incidents", studentSubject, "")
	if rec.Code != http.StatusOK {
		t.Errorf("own history status = %d, want 200", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/students/"+otherStudent+"/incidents", studentSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("other history status = %d, want 403", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/students/"+studentSubject+"/incidents", unknownSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("unknown subject status = %d, want 403", rec.Code)
	}

	rec = doRequest(t, h, http.MethodPost, "/v1/incidents", teacherSubject,
		`{"studentID":"`+studentSubject+`","classID":"9-1","severity":"tremenda","description":"X"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad severity status = %d, want 400", rec.Code)
	}
	if code := errorCode(t, rec); code != "incidents.err_unknown_severity" {
		t.Errorf("code = %q, want incidents.err_unknown_severity", code)
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/incidents", studentSubject, payload)
	if rec.Code != http.StatusForbidden {
		t.Errorf("student report status = %d, want 403", rec.Code)
	}
}

func TestDashboardSummaryAdminOnly(t *testing.T) {
	h := testStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/dashboard/summary?year=2026", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var summary struct {
		Year   int `json:"Year"`
		Totals struct {
			Teachers int `json:"Teachers"`
			Students int `json:"Students"`
			Groups   int `json:"Groups"`
		} `json:"Totals"`
		GroupsTruncated bool `json:"GroupsTruncated"`
		MarksLast7d     struct {
			Presences int `json:"Presences"`
		} `json:"MarksLast7d"`
		TopRisk        []any `json:"TopRisk"`
		RecentWarnings []any `json:"RecentWarnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.Year != 2026 {
		t.Errorf("Year = %d, want 2026", summary.Year)
	}
	if summary.Totals.Groups != 1 || summary.Totals.Students != 2 {
		t.Errorf("Totals = %+v, want 1 group and 2 students", summary.Totals)
	}
	if summary.GroupsTruncated {
		t.Error("GroupsTruncated = true, want false")
	}
	// The fixture call (today, no marks) leaves one presence behind.
	if summary.MarksLast7d.Presences != 1 {
		t.Errorf("Presences = %d, want 1", summary.MarksLast7d.Presences)
	}
	if summary.TopRisk == nil || summary.RecentWarnings == nil {
		t.Error("ranking and recents must be empty arrays, never nil")
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/dashboard/summary?year=2026", teacherSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("teacher status = %d, want 403", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/dashboard/summary", adminSubject, "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("missing year status = %d, want 400", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/dashboard/summary?year=2030", adminSubject, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown year status = %d, want 404", rec.Code)
	}
}
