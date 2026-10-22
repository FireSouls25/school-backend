package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"grade/src/core/classes"
	"grade/src/core/enrollments"
	"grade/src/core/roles"
	"grade/src/core/schoolyears"
	"grade/src/core/students"
	httpapi "grade/src/platform/http"
)

func enrollStack(t *testing.T) (http.Handler, *schoolyears.Service, *classes.Service, *students.Service) {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	yearsSvc := schoolyears.NewService(schoolyears.NewMemoryStore())
	classesSvc := classes.NewService(classes.NewMemoryStore())
	studentsSvc := students.NewService(students.NewMemoryStore())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	must(roleSvc.Assign(ctx, teacherSubject, roles.RoleTeacher))
	return httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		AllowDevIdentity: true,
		Years:            yearsSvc,
		Classes:          classesSvc,
		Students:         studentsSvc,
		Enrollments:      enrollments.NewService(enrollments.NewMemoryStore()),
	}, newI18n(t)), yearsSvc, classesSvc, studentsSvc
}

func seedYear(t *testing.T, yearsSvc *schoolyears.Service, year int) string {
	t.Helper()
	y, err := yearsSvc.Create(context.Background(), schoolyears.SchoolYear{
		Year: year, Periods: 2,
		StartDate: time.Date(year, 2, 2, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(year, 11, 27, 0, 0, 0, 0, time.UTC),
		PeriodRanges: []schoolyears.PeriodRange{
			{Start: time.Date(year, 2, 2, 0, 0, 0, 0, time.UTC), End: time.Date(year, 6, 12, 0, 0, 0, 0, time.UTC)},
			{Start: time.Date(year, 7, 6, 0, 0, 0, 0, time.UTC), End: time.Date(year, 11, 27, 0, 0, 0, 0, time.UTC)},
		},
	})
	if err != nil {
		t.Fatalf("seed year: %v", err)
	}
	return y.ID
}

func TestEnrollmentFlow(t *testing.T) {
	h, yearsSvc, classesSvc, studentsSvc := enrollStack(t)
	ctx := context.Background()
	y2026 := seedYear(t, yearsSvc, 2026)

	g, err := classesSvc.Create(ctx, classes.ClassGroup{SchoolYearID: y2026, Grade: 9, GroupNo: 1})
	if err != nil {
		t.Fatalf("seed group: %v", err)
	}
	st, err := studentsSvc.Create(ctx, validStudent(""))
	if err != nil {
		t.Fatalf("seed student: %v", err)
	}

	post := func(target, subject, body string) (int, map[string]any) {
		rec := doRequest(t, h, http.MethodPost, target, subject, body)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	if code, _ := post("/v1/enrollments", teacherSubject, `{"studentID":"x","classGroupID":"y"}`); code != http.StatusForbidden {
		t.Fatalf("teacher enroll = %d, want 403", code)
	}
	body := `{"studentID":"` + st.ID + `","classGroupID":"` + g.ID + `"}`
	if code, out := post("/v1/enrollments", adminSubject, body); code != http.StatusCreated {
		t.Fatalf("enroll = %d, want 201: %v", code, out)
	}
	rec := doRequest(t, h, http.MethodPost, "/v1/enrollments", adminSubject, body)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "enrollments.err_duplicate_enrollment" {
		t.Fatalf("duplicate = %d, want 400 duplicate: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/enrollments", adminSubject,
		`{"studentID":"00000000-0000-4000-8000-000000000000","classGroupID":"`+g.ID+`"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown student = %d, want 404", rec.Code)
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/enrollments?groupID="+g.ID, adminSubject, "")
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list by group = %s, want 1", rec.Body.String())
	}
	if rec := doRequest(t, h, http.MethodGet, "/v1/enrollments", adminSubject, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("unfiltered = %d, want 400", rec.Code)
	}
	if rec := doRequest(t, h, http.MethodGet, "/v1/enrollments?groupID="+g.ID+"&studentID="+st.ID, adminSubject, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("both filters = %d, want 400", rec.Code)
	}
	rec = doRequest(t, h, http.MethodDelete, "/v1/enrollments/"+list[0]["ID"].(string), adminSubject, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", rec.Code)
	}

	// Closed years take no enrollments.
	if _, err := yearsSvc.Close(ctx, y2026); err != nil {
		t.Fatalf("close: %v", err)
	}
	if code, out := post("/v1/enrollments", adminSubject, body); code != http.StatusBadRequest {
		t.Fatalf("closed enroll = %d, want 400: %v", code, out)
	}
}

func TestPromotionFlow(t *testing.T) {
	h, yearsSvc, classesSvc, studentsSvc := enrollStack(t)
	ctx := context.Background()
	y26 := seedYear(t, yearsSvc, 2026)
	y27 := seedYear(t, yearsSvc, 2027)

	g9, _ := classesSvc.Create(ctx, classes.ClassGroup{SchoolYearID: y26, Grade: 9, GroupNo: 1})
	g11, _ := classesSvc.Create(ctx, classes.ClassGroup{SchoolYearID: y26, Grade: 11, GroupNo: 1})
	g10next, _ := classesSvc.Create(ctx, classes.ClassGroup{SchoolYearID: y27, Grade: 10, GroupNo: 1})
	g11next, _ := classesSvc.Create(ctx, classes.ClassGroup{SchoolYearID: y27, Grade: 11, GroupNo: 1})
	st, _ := studentsSvc.Create(ctx, validStudent(""))
	senior, _ := studentsSvc.Create(ctx, students.Student{
		Names: "Luis", Surnames: "Pérez", ClassID: "11-1",
		DocumentID: "9988776655", Caregiver: students.Guardian{Names: "Ana Pérez"},
	})

	post := func(body string) (int, map[string]any) {
		rec := doRequest(t, h, http.MethodPost, "/v1/promotions", adminSubject, body)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	promote := func(student, from, to string) (int, map[string]any) {
		return post(`{"studentID":"` + student + `","fromGroupID":"` + from + `","toGroupID":"` + to + `","decision":"promote"}`)
	}

	// Promote enrolls the destination; a retry tolerates the duplicate.
	if code, _ := promote(st.ID, g9.ID, g10next.ID); code != http.StatusCreated {
		t.Fatalf("promote = %d, want 201", code)
	}
	rec := doRequest(t, h, http.MethodGet, "/v1/enrollments?groupID="+g10next.ID, adminSubject, "")
	var placed []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &placed)
	if len(placed) != 1 {
		t.Fatalf("destination roster = %s, want 1", rec.Body.String())
	}
	if code, _ := promote(st.ID, g9.ID, g10next.ID); code != http.StatusCreated {
		t.Fatalf("promote retry = %d, want 201", code)
	}

	// Repeat bumps the counter and enrolls the destination grade.
	rec = doRequest(t, h, http.MethodPost, "/v1/promotions", adminSubject,
		`{"studentID":"`+st.ID+`","fromGroupID":"`+g9.ID+`","toGroupID":"`+g10next.ID+`","decision":"repeat"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("repeat = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	updated, _ := studentsSvc.ByID(ctx, st.ID)
	if updated.RepeatCount != 1 {
		t.Fatalf("RepeatCount = %d, want 1", updated.RepeatCount)
	}

	// Graduate ends the lifecycle without destination.
	rec = doRequest(t, h, http.MethodPost, "/v1/promotions", adminSubject,
		`{"studentID":"`+senior.ID+`","fromGroupID":"`+g11.ID+`","decision":"graduate"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("graduate = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	grad, _ := studentsSvc.ByID(ctx, senior.ID)
	if string(grad.Status) != "graduated" {
		t.Fatalf("status = %q, want graduated", grad.Status)
	}

	// Grade rules: no promote from 11th, no graduate outside 11th.
	if code, _ := promote(senior.ID, g11.ID, g11next.ID); code != http.StatusBadRequest {
		t.Fatalf("promote from 11th = %d, want 400", code)
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/promotions", adminSubject,
		`{"studentID":"`+st.ID+`","fromGroupID":"`+g9.ID+`","decision":"graduate"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("graduate from 9th = %d, want 400", rec.Code)
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/promotions", adminSubject,
		`{"studentID":"`+senior.ID+`","fromGroupID":"`+g11.ID+`","toGroupID":"`+g11next.ID+`","decision":"graduate"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("graduate with destination = %d, want 400", rec.Code)
	}

	// Review: decisions from the origin group, history per student.
	rec = doRequest(t, h, http.MethodGet, "/v1/promotions?groupID="+g9.ID, adminSubject, "")
	var decided []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &decided)
	if len(decided) != 3 {
		t.Fatalf("group promotions = %s, want 3", rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/promotions?studentID="+senior.ID, adminSubject, "")
	var history []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &history)
	if len(history) != 1 {
		t.Fatalf("student promotions = %s, want 1", rec.Body.String())
	}
}
