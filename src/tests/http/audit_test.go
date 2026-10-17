package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"grade/src/core/audit"
	"grade/src/core/roles"
	"grade/src/core/schedules"
	httpapi "grade/src/platform/http"
)

func auditStack(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	must(roleSvc.Assign(ctx, teacherSubject, roles.RoleTeacher))
	return httpapi.Router(httpapi.Dependencies{
		Auth:      roleSvc,
		Roles:     roleSvc,
		RolesSvc:  roleSvc,
		Audit:     audit.NewService(audit.NewMemoryStore()),
		Schedules: schedules.NewService(schedules.NewMemoryStore(), allowAllChecker{}),
	}, newI18n(t))
}

func TestScheduleWritesAreTrailed(t *testing.T) {
	h := auditStack(t)

	// The feed is admin-only; teachers see the denied state, not rows.
	rec := doRequest(t, h, http.MethodGet, "/v1/audit?limit=10", teacherSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("teacher feed status = %d, want 403", rec.Code)
	}

	body := `{"classGroupID":"22222222-2222-4222-8222-222222222222","teacherID":"33333333-3333-4333-8333-333333333333","subjectID":"44444444-4444-4444-8444-444444444444","weekday":1,"start":"07:00","end":"08:00"}`
	rec = doRequest(t, h, http.MethodPost, "/v1/schedule", adminSubject, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}

	// A colliding write fails and trails its real outcome too.
	rec = doRequest(t, h, http.MethodPost, "/v1/schedule", adminSubject, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("conflict status = %d, want 400", rec.Code)
	}

	// Newest first: the 400 row, then the 201 row.
	rec = doRequest(t, h, http.MethodGet, "/v1/audit?limit=10", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("feed status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode feed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows len = %d, want 2", len(rows))
	}
	if rows[0]["Status"] != float64(400) || rows[0]["Action"] != "create" || rows[0]["Resource"] != "schedule" {
		t.Errorf("rows[0] = %v, want the 400 create", rows[0])
	}
	if rows[1]["Status"] != float64(201) {
		t.Errorf("rows[1] = %v, want the 201 create", rows[1])
	}
	if rows[1]["ResourceID"] != created["ID"] {
		t.Errorf("ResourceID = %v, want %v", rows[1]["ResourceID"], created["ID"])
	}

	// Student filter excludes schedule rows (they concern no student).
	rec = doRequest(t, h, http.MethodGet, "/v1/audit?studentID=55555555-5555-4555-8555-555555555555", adminSubject, "")
	var filtered []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &filtered); err != nil || len(filtered) != 0 {
		t.Fatalf("filtered = %v, %v; want empty", filtered, err)
	}

	// Bad limit is rejected.
	rec = doRequest(t, h, http.MethodGet, "/v1/audit?limit=nope", adminSubject, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad limit status = %d, want 400", rec.Code)
	}
}
