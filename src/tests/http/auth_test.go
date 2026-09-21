package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"grade/src/core/roles"
	"grade/src/core/sessions"
	"grade/src/core/statistics"
	"grade/src/core/students"
	"grade/src/core/teachers"
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
)

type fakeStats struct{}

func (fakeStats) GroupSessions(context.Context, string) ([]statistics.SessionView, error) {
	return nil, nil
}
func (fakeStats) StudentSessions(context.Context, string) ([]statistics.SessionView, error) {
	return nil, nil
}
func (fakeStats) StudentWarnings(context.Context, string) ([]statistics.WarningView, error) {
	return nil, nil
}
func (fakeStats) StudentFaults(context.Context, string) ([]statistics.FaultView, error) {
	return nil, nil
}

func validStudent(id string) students.Student {
	return students.Student{
		ID: id, Names: "Ana", Surnames: "Gómez", ClassID: "9-1",
		DocumentID: "1234567890",
		Caregiver:  students.Guardian{Names: "María Gómez"},
	}
}

func testStack(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()

	roleSvc := roles.NewService(roles.NewMemoryStore())
	studentsSvc := students.NewService(students.NewMemoryStore())
	teachersSvc := teachers.NewService(teachers.NewMemoryStore())
	warningsSvc := warnings.NewService(warnings.NewMemoryStore())
	sessionsSvc := sessions.NewService(sessions.NewMemoryStore())
	statsSvc := statistics.NewService(fakeStats{}, fakeStats{}, fakeStats{})

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	must(roleSvc.Assign(ctx, teacherSubject, roles.RoleTeacher))

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
	sess, err := sessionsSvc.OpenSession(ctx, sessions.Session{
		ClassGroupID: groupID,
		ClassLabel:   "9-1",
		SchoolYear:   2026,
		TeacherID:    teacherSubject,
		Date:         time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		Period:       2,
		Roster: []sessions.RosterEntry{
			{StudentID: studentSubject, Names: "Ana", Surnames: "Gómez", DocumentID: "1234567890"},
		},
	})
	must(err)
	sessionRouteID = sess.ID

	return httpapi.Router(httpapi.Dependencies{
		Auth:       roleSvc,
		Roles:      roleSvc,
		RolesSvc:   roleSvc,
		Students:   studentsSvc,
		Teachers:   teachersSvc,
		Warnings:   warningsSvc,
		Sessions:   sessionsSvc,
		Statistics: statsSvc,
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
