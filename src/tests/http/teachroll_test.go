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
	"grade/src/core/sessions"
	"grade/src/core/tattendance"
	"grade/src/core/teachers"
	"grade/src/core/users"
	"grade/src/core/warnings"
	httpapi "grade/src/platform/http"
)

type teachRollSetup struct {
	h           http.Handler
	profID      string
	otherID     string
	sessionsSvc *sessions.Service
	warningsSvc *warnings.Service
	usersSvc    *users.Service
	taSvc       *tattendance.Service
}

func teachRollStack(t *testing.T) teachRollSetup {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	teachersSvc := teachers.NewService(teachers.NewMemoryStore())
	schedulesSvc := schedules.NewService(schedules.NewMemoryStore(), allowAllChecker{})
	taSvc := tattendance.NewService(tattendance.NewMemoryStore())
	sessionsSvc := sessions.NewService(sessions.NewMemoryStore())
	warningsSvc := warnings.NewService(warnings.NewMemoryStore())
	usersSvc := users.NewServiceWithCost(users.NewMemoryStore(), 4)
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
		Sessions:         sessionsSvc,
		Warnings:         warningsSvc,
		Users:            usersSvc,
	}, newI18n(t))
	return teachRollSetup{
		h:           h,
		profID:      prof.ID,
		otherID:     other.ID,
		sessionsSvc: sessionsSvc,
		warningsSvc: warningsSvc,
		usersSvc:    usersSvc,
		taSvc:       taSvc,
	}
}

func TestTeacherRollFlow(t *testing.T) {
	st := teachRollStack(t)
	h, profID, otherID := st.h, st.profID, st.otherID
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
	st := teachRollStack(t)
	h, profID := st.h, st.profID

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

func TestTeacherActivityFlow(t *testing.T) {
	st := teachRollStack(t)
	h, profID := st.h, st.profID
	ctx := context.Background()

	// A roll call taken by the teacher, one absence recorded.
	sess, err := st.sessionsSvc.OpenSession(ctx, sessions.Session{
		ClassGroupID: "33333333-3333-4333-8333-333333333333",
		ClassLabel:   "9-1",
		SchoolYear:   2026,
		TeacherID:    profID,
		Date:         time.Now().UTC(),
		Period:       1,
		Roster: []sessions.RosterEntry{
			{StudentID: "44444444-4444-4444-8444-444444444444", Names: "Ana", Surnames: "Gómez"},
			{StudentID: "55555555-5555-4555-8555-555555555555", Names: "Luis", Surnames: "Pardo"},
		},
	})
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	if _, err := st.sessionsSvc.RecordMark(ctx, sess.ID, "55555555-5555-4555-8555-555555555555", sessions.MarkAbsence, profID, ""); err != nil {
		t.Fatalf("record mark: %v", err)
	}

	// One llamado issued from the teacher's linked login.
	login, err := st.usersSvc.Create(ctx, "profe", "", "profe123*")
	if err != nil {
		t.Fatalf("create login: %v", err)
	}
	if _, err := st.usersSvc.LinkTeacher(ctx, login.ID, profID); err != nil {
		t.Fatalf("link login: %v", err)
	}
	if _, err := st.warningsSvc.Issue(ctx, warnings.Input{
		StudentID:   "44444444-4444-4444-8444-444444444444",
		ClassID:     "9-1",
		TeacherID:   login.ID,
		HappenedAt:  time.Now().UTC(),
		Gravity:     warnings.GravitySevere,
		Title:       "Desorden",
		Description: "Interrumpió la clase.",
		Snapshot:    warnings.StudentSnapshot{Names: "Ana", Surnames: "Gómez", DocumentID: "123", ClassID: "9-1"},
	}); err != nil {
		t.Fatalf("issue warning: %v", err)
	}

	// Unknown teachers 404 on both feeds.
	for _, target := range []string{
		"/v1/teachers/00000000-0000-4000-8000-000000000000/sessions",
		"/v1/teachers/00000000-0000-4000-8000-000000000000/warnings-made",
	} {
		rec := doRequest(t, h, http.MethodGet, target, adminSubject, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404", target, rec.Code)
		}
	}

	rec := doRequest(t, h, http.MethodGet, "/v1/teachers/"+profID+"/sessions", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("sessions status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var taken []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &taken); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if len(taken) != 1 || taken[0]["Absences"] != float64(1) || taken[0]["Students"] != float64(2) {
		t.Fatalf("sessions = %v, want one roll with one absence of two", taken)
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/"+profID+"/warnings-made", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("warnings-made status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var made []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil {
		t.Fatalf("decode warnings-made: %v", err)
	}
	if len(made) != 1 || made[0]["Title"] != "Desorden" {
		t.Fatalf("warnings-made = %v, want the single llamado", made)
	}
}

func TestSessionsHistoryFlow(t *testing.T) {
	st := teachRollStack(t)
	h := st.h
	ctx := context.Background()

	groupID := "66666666-6666-4666-8666-666666666666"
	sess, err := st.sessionsSvc.OpenSession(ctx, sessions.Session{
		ClassGroupID: groupID,
		ClassLabel:   "9-1",
		SchoolYear:   2026,
		TeacherID:    st.profID,
		Date:         time.Now().UTC(),
		Period:       1,
		Roster: []sessions.RosterEntry{
			{StudentID: "44444444-4444-4444-8444-444444444444", Names: "Ana", Surnames: "Gómez"},
			{StudentID: "55555555-5555-4555-8555-555555555555", Names: "Luis", Surnames: "Pardo"},
		},
	})
	if err != nil {
		t.Fatalf("open session: %v", err)
	}
	if _, err := st.sessionsSvc.RecordMark(ctx, sess.ID, "44444444-4444-4444-8444-444444444444", sessions.MarkLate, st.profID, "Bus"); err != nil {
		t.Fatalf("record mark: %v", err)
	}
	if _, err := st.sessionsSvc.RecordMark(ctx, sess.ID, "55555555-5555-4555-8555-555555555555", sessions.MarkEvasion, st.profID, ""); err != nil {
		t.Fatalf("record evasion: %v", err)
	}

	// Salon is required: no unfiltered dumps.
	rec := doRequest(t, h, http.MethodGet, "/v1/sessions/history", adminSubject, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unfiltered status = %d, want 400", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/sessions/history?classGroupID="+groupID, adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("history status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var cards []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cards); err != nil {
		t.Fatalf("decode cards: %v", err)
	}
	if len(cards) != 1 || cards[0]["Late"] != float64(1) || cards[0]["Evasion"] != float64(1) || cards[0]["Present"] != float64(0) || cards[0]["ClassLabel"] != "9-1" {
		t.Fatalf("cards = %v, want the late+evasion roll", cards)
	}
	if cards[0]["Teacher"] == "" {
		t.Fatalf("cards = %v, want the teacher resolved", cards)
	}

	// Detail with marks and the revision trail.
	rec = doRequest(t, h, http.MethodGet, "/v1/sessions/"+sess.ID, adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var detail struct {
		Marks     map[string]string `json:"Marks"`
		Revisions []map[string]any  `json:"Revisions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if len(detail.Revisions) != 2 || detail.Revisions[0]["To"] != "late" || detail.Revisions[1]["To"] != "evasion" {
		t.Fatalf("detail = %+v, want late then evasion oldest first", detail)
	}
	if detail.Revisions[0]["Student"] != "Ana Gómez" || detail.Revisions[0]["Note"] != "Bus" {
		t.Fatalf("detail = %+v, want resolved student and note", detail)
	}
}
