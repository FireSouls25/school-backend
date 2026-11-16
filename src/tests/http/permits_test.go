package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// cot mirrors the server's school zone: Colombia, fixed UTC-5.
var cot = time.FixedZone("COT", -5*60*60)

// todayISO is the day the permission endpoints normalize against: the
// school's wall clock (Colombia, UTC-5), never UTC, so an evening class
// keeps the same calendar day the office used when granting the permit.
func todayISO() string {
	return time.Now().In(cot).Format("2006-01-02")
}

// TestPriorPermitsFlow covers grant, listing by group and by student,
// validation and revocation through the HTTP surface.
func TestPriorPermitsFlow(t *testing.T) {
	h := testStack(t)
	day := todayISO()

	body := `{"studentID":"` + studentSubject + `","classGroupID":"` + realGroupID +
		`","date":"` + day + `","reason":"Cita médica"}`
	rec := doRequest(t, h, http.MethodPost, "/v1/attendance/permits", adminSubject, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("grant status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var permit struct {
		ID           string
		StudentID    string
		ClassGroupID string
		ClassLabel   string
		Reason       string
		CreatedBy    string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &permit); err != nil {
		t.Fatalf("decode permit: %v", err)
	}
	if permit.ID == "" || permit.StudentID != studentSubject || permit.Reason != "Cita médica" {
		t.Fatalf("unexpected permit: %+v", permit)
	}
	if permit.ClassLabel != "9-1" {
		t.Errorf("class label = %q, want 9-1", permit.ClassLabel)
	}

	// The teacher sees the permits of the group/day to lock the roster.
	rec = doRequest(t, h, http.MethodGet,
		"/v1/attendance/permits?classGroupID="+realGroupID+"&date="+day, teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("group permits status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var groupList []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &groupList); err != nil {
		t.Fatalf("decode group permits: %v", err)
	}
	if len(groupList) != 1 {
		t.Fatalf("group permits len = %d, want 1", len(groupList))
	}

	// The student file lists the same permit by student.
	rec = doRequest(t, h, http.MethodGet,
		"/v1/attendance/permits?studentID="+studentSubject, adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("student permits status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var studentList []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &studentList); err != nil {
		t.Fatalf("decode student permits: %v", err)
	}
	if len(studentList) != 1 {
		t.Fatalf("student permits len = %d, want 1", len(studentList))
	}

	// An unexplained permission is rejected: it would be an
	// indistinguishable absence.
	rec = doRequest(t, h, http.MethodPost, "/v1/attendance/permits", adminSubject,
		`{"studentID":"`+studentSubject+`","classGroupID":"`+realGroupID+`","date":"`+day+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty reason status = %d, want 400", rec.Code)
	}
	if code := errorCode(t, rec); code != "permits.err_empty_reason" {
		t.Errorf("code = %q, want permits.err_empty_reason", code)
	}

	// Teachers cannot grant nor revoke: manage-permits belongs to the
	// office, even though teachers now record defenses.
	rec = doRequest(t, h, http.MethodPost, "/v1/attendance/permits", teacherSubject, body)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("teacher grant status = %d, want 403", rec.Code)
	}
	rec = doRequest(t, h, http.MethodDelete, "/v1/attendance/permits/"+permit.ID, teacherSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("teacher revoke status = %d, want 403", rec.Code)
	}

	// Revoking removes the row; a second revoke reports it is gone.
	rec = doRequest(t, h, http.MethodDelete, "/v1/attendance/permits/"+permit.ID, adminSubject, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodDelete, "/v1/attendance/permits/"+permit.ID, adminSubject, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second revoke status = %d, want 404", rec.Code)
	}
}

// TestPriorPermitLocksMarks proves the permission outranks the teacher:
// present, absence, late and evasion are rejected; only the permission
// mark itself is accepted.
func TestPriorPermitLocksMarks(t *testing.T) {
	h := testStack(t)
	day := todayISO()

	body := `{"studentID":"` + studentSubject + `","classGroupID":"` + realGroupID +
		`","date":"` + day + `","reason":"Trámite familiar"}`
	rec := doRequest(t, h, http.MethodPost, "/v1/attendance/permits", adminSubject, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("grant status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	mark := func(value string) *httptest.ResponseRecorder {
		return doRequest(t, h, http.MethodPost,
			"/v1/sessions/"+sessionRouteID+"/marks", teacherSubject,
			`{"studentID":"`+studentSubject+`","mark":"`+value+`"}`)
	}

	for _, value := range []string{"absence", "late", "evasion", ""} {
		rec = mark(value)
		if rec.Code != http.StatusConflict {
			t.Errorf("mark %q status = %d, want 409: %s", value, rec.Code, rec.Body.String())
			continue
		}
		if code := errorCode(t, rec); code != "permits.err_locked" {
			t.Errorf("mark %q code = %q, want permits.err_locked", value, code)
		}
	}

	// The permission mark stays allowed; if the auto-permit already set
	// it, the server reports "no change" instead of rewriting it.
	rec = mark("absence-permission")
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
		t.Fatalf("permission mark status = %d, want 200/201/400: %s", rec.Code, rec.Body.String())
	}
	if rec.Code == http.StatusBadRequest {
		if code := errorCode(t, rec); code != "sessions.err_no_change" {
			t.Errorf("code = %q, want sessions.err_no_change", code)
		}
	}
}

// TestOmittedRollcalls lists the blocks already ended without a call and
// stops listing a block once its session is opened.
func TestOmittedRollcalls(t *testing.T) {
	h := testStack(t)

	// A past Monday inside the 2026 school year: the fixture schedule has
	// exactly one block that day and no session yet.
	day := "2026-01-19"
	rec := doRequest(t, h, http.MethodGet, "/v1/attendance/omitted?date="+day, teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("omitted status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var list []struct {
		ClassGroupID string
		ClassLabel   string
		SubjectID    string
		Omitted      bool
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode omitted: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("omitted len = %d, want 1: %s", len(list), rec.Body.String())
	}
	if !list[0].Omitted || list[0].ClassLabel != "9-1" {
		t.Errorf("unexpected omitted row: %+v", list[0])
	}

	// A future day has nothing to omit.
	rec = doRequest(t, h, http.MethodGet, "/v1/attendance/omitted?date=2030-01-21", teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("future omitted status = %d, want 200", rec.Code)
	}
	var empty []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil {
		t.Fatalf("decode future omitted: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("future omitted len = %d, want 0", len(empty))
	}

	// Calling the class removes it from the omitted list. Noon UTC is
	// 07:00 in the school zone, so the session keeps the queried day.
	rec = doRequest(t, h, http.MethodPost, "/v1/sessions/open", teacherSubject,
		`{"classGroupID":"`+realGroupID+`","period":1,"date":"2026-01-19T12:00:00Z","subjectID":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("open session status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/attendance/omitted?date="+day, teacherSubject, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode omitted after call: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("omitted len after call = %d, want 0: %s", len(list), rec.Body.String())
	}
}
