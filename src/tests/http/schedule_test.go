package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"grade/src/core/roles"
	"grade/src/core/schedules"
	"grade/src/core/subjects"
	"grade/src/core/teachers"
	httpapi "grade/src/platform/http"
)

func scheduleStack(t *testing.T) http.Handler {
	t.Helper()
	// The shared stack wires schedules with an allow-all pairing checker
	// and grants both roles; conflict and hours policy still applies.
	h := testStack(t)
	return h
}

func TestScheduleFlow(t *testing.T) {
	h := scheduleStack(t)
	_ = roles.RoleAdmin // roles granted in testStack

	teacherB := "99999999-9999-4999-8999-999999999999"

	// Reads need a filter; teachers may read, nobody dumps everything.
	rec := doRequest(t, h, http.MethodGet, "/v1/schedule", teacherSubject, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unfiltered status = %d, want 400", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/schedule?teacherID="+teacherSubject, teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("teacher read status = %d, want 200", rec.Code)
	}

	// Writes are admin-only.
	rec = doRequest(t, h, http.MethodPost, "/v1/schedule", teacherSubject,
		`{"classGroupID":"`+realGroupID+`","teacherID":"`+teacherSubject+`","subjectID":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","weekday":3,"start":"07:00","end":"08:00"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("teacher write status = %d, want 403", rec.Code)
	}

	// Bad clocks and out-of-hours fail before conflicts are even checked.
	for _, tc := range []struct{ name, body string }{
		{"bad clock", `{"classGroupID":"` + realGroupID + `","teacherID":"` + teacherSubject + `","subjectID":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","weekday":1,"start":"7am","end":"08:00"}`},
		{"too early", `{"classGroupID":"` + realGroupID + `","teacherID":"` + teacherSubject + `","subjectID":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","weekday":1,"start":"06:00","end":"07:00"}`},
		{"too late", `{"classGroupID":"` + realGroupID + `","teacherID":"` + teacherSubject + `","subjectID":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","weekday":1,"start":"13:00","end":"15:00"}`},
		{"too short", `{"classGroupID":"` + realGroupID + `","teacherID":"` + teacherSubject + `","subjectID":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","weekday":1,"start":"08:00","end":"08:30"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, http.MethodPost, "/v1/schedule", adminSubject, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
		})
	}

	// Place a slot, then collide with it both ways.
	rec = doRequest(t, h, http.MethodPost, "/v1/schedule", adminSubject,
		`{"classGroupID":"`+realGroupID+`","teacherID":"`+teacherSubject+`","subjectID":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","weekday":3,"start":"07:00","end":"08:00"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	entryID, _ := created["ID"].(string)
	if entryID == "" {
		t.Fatalf("created = %v, want id", created)
	}

	rec = doRequest(t, h, http.MethodPost, "/v1/schedule", adminSubject,
		`{"classGroupID":"`+realGroupID+`","teacherID":"`+teacherB+`","subjectID":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","weekday":3,"start":"07:30","end":"08:30"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("class overlap status = %d, want 400", rec.Code)
	}
	var envelope map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if envelope["error"]["code"] != "schedules.err_class_conflict" {
		t.Errorf("code = %q, want class conflict", envelope["error"]["code"])
	}

	// Filters: by group, by teacher, by weekday, combined.
	for _, target := range []string{
		"/v1/schedule?classGroupID=" + realGroupID,
		"/v1/schedule?teacherID=" + teacherSubject,
		"/v1/schedule?classGroupID=" + realGroupID + "&weekday=3",
		"/v1/schedule?classGroupID=" + realGroupID + "&weekday=2",
		"/v1/schedule?teacherID=" + teacherSubject + "&classGroupID=" + realGroupID,
	} {
		rec = doRequest(t, h, http.MethodGet, target, adminSubject, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", target, rec.Code)
		}
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/schedule?classGroupID="+realGroupID+"&weekday=2", adminSubject, "")
	var empty []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil || len(empty) != 0 {
		t.Fatalf("tuesday filter = %v, %v; want empty", empty, err)
	}

	// Update keeps the slot; delete removes it.
	rec = doRequest(t, h, http.MethodPut, "/v1/schedule/"+entryID, adminSubject,
		`{"classGroupID":"`+realGroupID+`","teacherID":"`+teacherSubject+`","subjectID":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","weekday":3,"start":"08:00","end":"09:00"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodDelete, "/v1/schedule/"+entryID, adminSubject, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
}

func TestScheduleAutoAssignsPairing(t *testing.T) {
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	subjectsSvc := subjects.NewService(subjects.NewMemoryStore())
	schedulesSvc := schedules.NewService(schedules.NewMemoryStore(), allowAllChecker{})
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	subj, err := subjectsSvc.CreateSubject(ctx, subjects.Subject{Name: "Física"})
	must(err)
	h := httpapi.Router(httpapi.Dependencies{
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		AllowDevIdentity: true,
		Schedules:        schedulesSvc,
		Subjects:         subjectsSvc,
		Teachers:         teachers.NewService(teachers.NewMemoryStore()),
	}, newI18n(t))

	// No pairing exists, yet placing the class succeeds: the pairing is
	// assigned automatically instead of failing untaught.
	rec := doRequest(t, h, http.MethodPost, "/v1/schedule", adminSubject,
		`{"classGroupID":"`+realGroupID+`","teacherID":"`+teacherSubject+`","subjectID":"`+subj.ID+`","weekday":2,"start":"09:00","end":"10:00"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/"+teacherSubject+"/assignments", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("timeline status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var timeline []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &timeline); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	if len(timeline) != 1 || timeline[0]["SubjectID"] != subj.ID {
		t.Fatalf("timeline = %v, want the single auto-assigned pairing", timeline)
	}
}
