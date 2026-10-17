package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"grade/src/core/roles"
	"grade/src/core/schedules"
	"grade/src/core/users"
	httpapi "grade/src/platform/http"
)

func myScheduleStack(t *testing.T) (http.Handler, *users.Service, *schedules.Service, *roles.Service) {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	usersSvc := users.NewServiceWithCost(users.NewMemoryStore(), bcrypt.MinCost)
	schedulesSvc := schedules.NewService(schedules.NewMemoryStore(), allowAllChecker{})
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	return httpapi.Router(httpapi.Dependencies{
		Auth:      roleSvc,
		Roles:     roleSvc,
		RolesSvc:  roleSvc,
		Users:     usersSvc,
		Schedules: schedulesSvc,
	}, newI18n(t)), usersSvc, schedulesSvc, roleSvc
}

func TestMyScheduleFollowsTeacherLink(t *testing.T) {
	h, usersSvc, schedulesSvc, roleSvc := myScheduleStack(t)
	ctx := context.Background()

	teacherID := "11111111-1111-4111-8111-111111111111"
	if _, err := schedulesSvc.Create(ctx, schedules.Entry{
		ClassGroupID: "22222222-2222-4222-8222-222222222222",
		TeacherID:    teacherID,
		SubjectID:    "33333333-3333-4333-8333-333333333333",
		Weekday:      int(time.Monday), Start: schedules.MustClock("07:00"), End: schedules.MustClock("08:00"),
	}); err != nil {
		t.Fatalf("seed entry: %v", err)
	}

	account, err := usersSvc.Create(ctx, "carlos.mendoza", "carlos@colegio.edu", "docente123*")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := roleSvc.Assign(ctx, account.ID, roles.RoleTeacher); err != nil {
		t.Fatalf("grant teacher: %v", err)
	}

	// Unlinked: the subject itself is tried, so nothing resolves.
	rec := doRequest(t, h, http.MethodGet, "/v1/teachers/me/schedule?weekday=1", account.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("unlinked status = %d, want 200", rec.Code)
	}
	var empty []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil || len(empty) != 0 {
		t.Fatalf("unlinked body = %v, %v; want empty", empty, err)
	}

	// Linked: the teacher's Monday slot resolves through the account.
	if _, err := usersSvc.LinkTeacher(ctx, account.ID, teacherID); err != nil {
		t.Fatalf("link: %v", err)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/me/schedule?weekday=1", account.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("linked status = %d, want 200", rec.Code)
	}
	var slots []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &slots); err != nil {
		t.Fatalf("decode slots: %v", err)
	}
	if len(slots) != 1 {
		t.Fatalf("slots len = %d, want 1", len(slots))
	}

	// Other weekdays stay empty: the link resolves, the day filters.
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/me/schedule?weekday=2", account.ID, "")
	var tuesday []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &tuesday); err != nil || len(tuesday) != 0 {
		t.Fatalf("tuesday = %v, %v; want empty", tuesday, err)
	}
}
