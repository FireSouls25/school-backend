package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"grade/src/core/roles"
	"grade/src/core/tattendance"
	"grade/src/core/teachers"
	httpapi "grade/src/platform/http"
)

func tattendanceStack(t *testing.T) (http.Handler, string) {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	teachersSvc := teachers.NewService(teachers.NewMemoryStore())
	taSvc := tattendance.NewService(tattendance.NewMemoryStore())
	if err := roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := roleSvc.Assign(ctx, teacherSubject, roles.RoleTeacher); err != nil {
		t.Fatalf("setup: %v", err)
	}
	prof, err := teachersSvc.Create(ctx, teachers.Teacher{
		Names: "Carlos", Surnames: "Mendoza", DocumentID: "79800111",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	h := httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		AllowDevIdentity: true,
		Teachers:         teachersSvc,
		TAttendance:      taSvc,
	}, newI18n(t))
	return h, prof.ID
}

func TestTeacherAttendanceFlow(t *testing.T) {
	h, profID := tattendanceStack(t)
	today := time.Now().UTC().Format("2006-01-02")

	// Teachers cannot take the roll: admin-only.
	rec := doRequest(t, h, http.MethodPost, "/v1/teachers/attendance", teacherSubject,
		fmt.Sprintf(`{"day":%q,"marks":[{"teacherID":%q}]}`, today, profID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("teacher record status = %d, want 403", rec.Code)
	}

	// Unknown teachers fail the whole day.
	rec = doRequest(t, h, http.MethodPost, "/v1/teachers/attendance", adminSubject,
		fmt.Sprintf(`{"day":%q,"marks":[{"teacherID":"00000000-0000-4000-8000-000000000000"}]}`, today))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown teacher status = %d, want 404", rec.Code)
	}

	// Future days are rejected: the roll records what happened.
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	rec = doRequest(t, h, http.MethodPost, "/v1/teachers/attendance", adminSubject,
		fmt.Sprintf(`{"day":%q,"marks":[{"teacherID":%q}]}`, tomorrow, profID))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("future day status = %d, want 400", rec.Code)
	}

	// Record: empty status means present, Spanish terms parse.
	rec = doRequest(t, h, http.MethodPost, "/v1/teachers/attendance", adminSubject,
		fmt.Sprintf(`{"day":%q,"marks":[{"teacherID":%q,"status":"ausente","note":"Permiso"}]}`, today, profID))
	if rec.Code != http.StatusOK {
		t.Fatalf("record status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Day roll carries display names.
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/attendance?date="+today, adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("day status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var day []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &day); err != nil {
		t.Fatalf("decode day: %v", err)
	}
	if len(day) != 1 || day[0]["Status"] != "absent" || day[0]["TeacherName"] != "Carlos Mendoza" {
		t.Fatalf("day = %v, want the absent Carlos Mendoza", day)
	}

	// History with summary counts.
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/"+profID+"/attendance", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("history status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var hist struct {
		Marks   []map[string]any `json:"Marks"`
		Summary map[string]int   `json:"Summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &hist); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(hist.Marks) != 1 || hist.Summary["Absent"] != 1 {
		t.Fatalf("history = %+v, want one absent mark", hist)
	}

	// Re-recording the day corrects it.
	rec = doRequest(t, h, http.MethodPost, "/v1/teachers/attendance", adminSubject,
		fmt.Sprintf(`{"day":%q,"marks":[{"teacherID":%q,"status":"presente"}]}`, today, profID))
	if rec.Code != http.StatusOK {
		t.Fatalf("correct status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/attendance?date="+today, adminSubject, "")
	var day2 []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &day2); err != nil {
		t.Fatalf("decode day2: %v", err)
	}
	if len(day2) != 1 || day2[0]["Status"] != "present" {
		t.Fatalf("day2 = %v, want the corrected present", day2)
	}
}
