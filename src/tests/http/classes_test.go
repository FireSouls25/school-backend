package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"grade/src/core/audit"
	"grade/src/core/classes"
	"grade/src/core/roles"
	"grade/src/core/schoolyears"
	httpapi "grade/src/platform/http"
)

func classesStack(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	yearsSvc := schoolyears.NewService(schoolyears.NewMemoryStore())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	must(roleSvc.Assign(ctx, adminSubject, roles.RoleAdmin))
	must(roleSvc.Assign(ctx, teacherSubject, roles.RoleTeacher))
	_, err := yearsSvc.Create(ctx, schoolyears.SchoolYear{
		Year: 2026, Periods: 3,
		StartDate: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC),
	})
	must(err)
	return httpapi.Router(httpapi.Dependencies{
		Auth:     roleSvc,
		Roles:    roleSvc,
		RolesSvc: roleSvc,
		Classes:  classes.NewService(classes.NewMemoryStore()),
		Years:    yearsSvc,
		Audit:    audit.NewService(audit.NewMemoryStore()),
	}, newI18n(t))
}

func TestCreateClassFlow(t *testing.T) {
	h := classesStack(t)

	// Teachers read salons but cannot create them.
	rec := doRequest(t, h, http.MethodPost, "/v1/classes", teacherSubject,
		`{"year":2026,"grade":10,"groupNo":1}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("teacher create status = %d, want 403", rec.Code)
	}

	// Validation carries stable codes.
	for _, tc := range []struct{ name, body, code string }{
		{"bad grade", `{"year":2026,"grade":12,"groupNo":1}`, "classes.err_invalid_grade"},
		{"bad group", `{"year":2026,"grade":10,"groupNo":0}`, "classes.err_invalid_group"},
		{"unknown year", `{"year":2030,"grade":10,"groupNo":1}`, "schoolyears.err_not_found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, http.MethodPost, "/v1/classes", adminSubject, tc.body)
			if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 400/404", rec.Code)
			}
			var envelope map[string]map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if envelope["error"]["code"] != tc.code {
				t.Errorf("code = %q, want %q", envelope["error"]["code"], tc.code)
			}
		})
	}

	// Create, then duplicate, then read back through the year listing.
	rec = doRequest(t, h, http.MethodPost, "/v1/classes", adminSubject,
		`{"year":2026,"grade":10,"groupNo":1}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/classes", adminSubject,
		`{"year":2026,"grade":10,"groupNo":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate status = %d, want 400", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/classes?year=2026", adminSubject, "")
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v; want the new salon", list, err)
	}

	// Both the success and the duplicate trailed with their outcomes.
	rec = doRequest(t, h, http.MethodGet, "/v1/audit?limit=10", adminSubject, "")
	var trail []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &trail); err != nil {
		t.Fatalf("decode trail: %v", err)
	}
	if len(trail) != 5 || trail[0]["Status"] != float64(400) || trail[1]["Status"] != float64(201) {
		t.Fatalf("trail = %v, want 400 over 201", trail)
	}
	if trail[0]["Resource"] != "group" {
		t.Errorf("resource = %v, want group", trail[0]["Resource"])
	}
}
