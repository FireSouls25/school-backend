package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"grade/src/core/roles"
	"grade/src/core/schedules"
	"grade/src/core/tattendance"
	"grade/src/core/teachers"
	httpapi "grade/src/platform/http"
)

func teachRollStack(t *testing.T) (http.Handler, string, string) {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	teachersSvc := teachers.NewService(teachers.NewMemoryStore())
	schedulesSvc := schedules.NewService(schedules.NewMemoryStore(), allowAllChecker{})
	taSvc := tattendance.NewService(tattendance.NewMemoryStore())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	must(roleSvc.Assign(ctx, teacherSubject, roles.RoleTeacher))
	prof, err := teachersSvc.Create(ctx, teachers.Teacher{
		Names: "Carlos", Surnames: "Mendoza", DocumentID: "79800111",
	})
	must(err)
	other, err := teachersSvc.Create(ctx, teachers.Teacher{
		Names: "Ana", Surnames: "Ríos", DocumentID: "79800222",
	})
	must(err)
	// Only prof teaches on Mondays; other has no entries at all.
	_, err = schedulesSvc.Create(ctx, schedules.Entry{
		ClassGroupID: "11111111-1111-4111-8111-111111111111",
		TeacherID:    prof.ID,
		SubjectID:    "22222222-2222-4222-8222-222222222222",
		Weekday:      1,
		Start:        420,
		End:          480,
	})
	must(err)
	h := httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		AllowDevIdentity: true,
		Teachers:         teachersSvc,
		Schedules:        schedulesSvc,
		TAttendance:      taSvc,
	}, newI18n(t))
	return h, prof.ID, other.ID
}

func TestTeacherRollFlow(t *testing.T) {
	h, profID, otherID := teachRollStack(t)
	// Most recent Monday: always today or past, never a future day.
	now := time.Now().UTC()
	delta := (int(now.Weekday()) + 6) % 7
	monday := now.AddDate(0, 0, -delta).Format("2006-01-02")
	fromParam := now.AddDate(0, 0, -delta-60).Format("2006-01-02")
	toParam := monday

	// Teachers cannot see the roll.
	rec := doRequest(t, h, http.MethodGet, "/v1/teachers/roll?date="+monday, teacherSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("teacher roll status = %d, want 403", rec.Code)
	}

	// Only the expected teacher is listed, nothing pre-selected.
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/roll?date="+monday, adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("roll status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var roll []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &roll); err != nil {
		t.Fatalf("decode roll: %v", err)
	}
	if len(roll) != 1 || roll[0]["TeacherID"] != profID {
		t.Fatalf("roll = %v, want only prof", roll)
	}
	if roll[0]["Recorded"] != false || roll[0]["Status"] != "" {
		t.Fatalf("roll row = %v, want unmarked", roll[0])
	}
	_ = otherID

	// Record late with a note.
	rec = doRequest(t, h, http.MethodPost, "/v1/teachers/attendance", adminSubject,
		fmt.Sprintf(`{"day":%q,"marks":[{"teacherID":%q,"status":"tarde","note":"Trancón"}]}`, monday, profID))
	if rec.Code != http.StatusOK {
		t.Fatalf("record status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/roll?date="+monday, adminSubject, "")
	var roll2 []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &roll2); err != nil {
		t.Fatalf("decode roll2: %v", err)
	}
	if len(roll2) != 1 || roll2[0]["Recorded"] != true || roll2[0]["Status"] != "late" {
		t.Fatalf("roll2 = %v, want the recorded late", roll2)
	}

	// History cards: one day with its summary.
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/attendance/history?from="+fromParam+"&to="+toParam, adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("range status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var cards []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cards); err != nil {
		t.Fatalf("decode cards: %v", err)
	}
	if len(cards) != 1 || cards[0]["Day"] != monday || cards[0]["Late"] != float64(1) || cards[0]["Total"] != float64(1) {
		t.Fatalf("cards = %v, want the single late day", cards)
	}

	// Correcting the day appends a revision with before and after.
	rec = doRequest(t, h, http.MethodPost, "/v1/teachers/attendance", adminSubject,
		fmt.Sprintf(`{"day":%q,"marks":[{"teacherID":%q,"status":"presente"}]}`, monday, profID))
	if rec.Code != http.StatusOK {
		t.Fatalf("correct status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/"+profID+"/attendance?from="+fromParam+"&to="+toParam, adminSubject, "")
	var hist struct {
		Marks     []map[string]any `json:"Marks"`
		Summary   map[string]int   `json:"Summary"`
		Revisions []map[string]any `json:"Revisions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &hist); err != nil {
		t.Fatalf("decode hist: %v", err)
	}
	if len(hist.Revisions) != 2 {
		t.Fatalf("revisions = %v, want first mark + correction", hist.Revisions)
	}
	last := hist.Revisions[0]
	if last["OldStatus"] != "late" || last["NewStatus"] != "present" {
		t.Fatalf("last revision = %v, want late -> present", last)
	}
}

func TestTeacherPhotoFlow(t *testing.T) {
	h, profID, _ := teachRollStack(t)

	// Empty photo is an honest 404, teachers cannot upload.
	rec := doRequest(t, h, http.MethodGet, "/v1/teachers/"+profID+"/photo", adminSubject, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("empty photo status = %d, want 404", rec.Code)
	}
	rec = doRequest(t, h, http.MethodPut, "/v1/teachers/"+profID+"/photo", teacherSubject, `{"data":"aGk="}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("teacher upload status = %d, want 403", rec.Code)
	}

	rec = doRequest(t, h, http.MethodPut, "/v1/teachers/"+profID+"/photo", adminSubject, `{"data":"aGk="}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("upload status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/"+profID+"/photo", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get photo status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode photo: %v", err)
	}
	if out["data"] != "aGk=" {
		t.Fatalf("photo = %v, want the uploaded bytes", out)
	}
}
