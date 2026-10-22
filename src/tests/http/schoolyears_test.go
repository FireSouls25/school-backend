package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"grade/src/core/classes"
	"grade/src/core/roles"
	"grade/src/core/schoolyears"
	httpapi "grade/src/platform/http"
)

func yearsStack(t *testing.T) http.Handler {
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
		Auth:             roleSvc,
		Roles:            roleSvc,
		RolesSvc:         roleSvc,
		AllowDevIdentity: true,
		Years:            schoolyears.NewService(schoolyears.NewMemoryStore()),
		Classes:          classes.NewService(classes.NewMemoryStore()),
	}, newI18n(t))
}

const yearBody = `{"year":2026,"periods":3,"startDate":"2026-02-02T00:00:00Z","endDate":"2026-11-27T00:00:00Z",` +
	`"holidays":["2026-05-01T00:00:00Z"],"periodRanges":[` +
	`{"start":"2026-02-02T00:00:00Z","end":"2026-04-17T00:00:00Z"},` +
	`{"start":"2026-04-20T00:00:00Z","end":"2026-07-17T00:00:00Z"},` +
	`{"start":"2026-07-27T00:00:00Z","end":"2026-11-27T00:00:00Z"}]}`

func createYear(t *testing.T, h http.Handler, subject, body string) (int, map[string]any) {
	t.Helper()
	rec := doRequest(t, h, http.MethodPost, "/v1/schoolyears", subject, body)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestSchoolYearFlow(t *testing.T) {
	h := yearsStack(t)

	// Teachers read, admins write.
	if rec := doRequest(t, h, http.MethodPost, "/v1/schoolyears", teacherSubject, yearBody); rec.Code != http.StatusForbidden {
		t.Fatalf("teacher create = %d, want 403", rec.Code)
	}
	if rec := doRequest(t, h, http.MethodGet, "/v1/schoolyears", teacherSubject, ""); rec.Code != http.StatusOK {
		t.Fatalf("teacher list = %d, want 200", rec.Code)
	}

	// New years always date their periods.
	if code, _ := createYear(t, h, adminSubject, `{"year":2026,"periods":3,"startDate":"2026-02-02T00:00:00Z","endDate":"2026-11-27T00:00:00Z"}`); code != http.StatusBadRequest {
		t.Fatalf("ranges-less create = %d, want 400", code)
	}

	code, created := createYear(t, h, adminSubject, yearBody)
	if code != http.StatusCreated {
		t.Fatalf("create = %d, want 201: %v", code, created)
	}
	id, _ := created["ID"].(string)
	if id == "" {
		t.Fatalf("body = %v, want ID", created)
	}

	// Overlapping ranges fail with a stable code.
	overlap := `{"year":2027,"periods":2,"startDate":"2027-02-01T00:00:00Z","endDate":"2027-11-26T00:00:00Z",` +
		`"periodRanges":[{"start":"2027-02-01T00:00:00Z","end":"2027-06-30T00:00:00Z"},` +
		`{"start":"2027-06-15T00:00:00Z","end":"2027-11-26T00:00:00Z"}]}`
	if rec := doRequest(t, h, http.MethodPost, "/v1/schoolyears", adminSubject, overlap); rec.Code != http.StatusBadRequest {
		t.Fatalf("overlap create = %d, want 400", rec.Code)
	}

	// List carries the dated periods; get resolves one year.
	rec := doRequest(t, h, http.MethodGet, "/v1/schoolyears", adminSubject, "")
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list = %s, want 1 year", rec.Body.String())
	}
	if pr, _ := list[0]["PeriodRanges"].([]any); len(pr) != 3 {
		t.Fatalf("PeriodRanges = %v, want 3", list[0]["PeriodRanges"])
	}
	if rec := doRequest(t, h, http.MethodGet, "/v1/schoolyears/"+id, adminSubject, ""); rec.Code != http.StatusOK {
		t.Fatalf("get = %d, want 200", rec.Code)
	}

	// Once the year grows salons, periods and dates freeze; holidays move.
	if rec := doRequest(t, h, http.MethodPost, "/v1/classes", adminSubject, `{"year":2026,"grade":9,"groupNo":1}`); rec.Code != http.StatusCreated {
		t.Fatalf("class create = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	frozen := `{"year":2026,"periods":4,"startDate":"2026-02-02T00:00:00Z","endDate":"2026-11-27T00:00:00Z",` +
		`"periodRanges":[{"start":"2026-02-02T00:00:00Z","end":"2026-11-27T00:00:00Z"}]}`
	if rec := doRequest(t, h, http.MethodPut, "/v1/schoolyears/"+id, adminSubject, frozen); rec.Code != http.StatusBadRequest {
		t.Fatalf("frozen edit = %d, want 400", rec.Code)
	}
	holidays := `{"year":2026,"periods":3,"startDate":"2026-02-02T00:00:00Z","endDate":"2026-11-27T00:00:00Z",` +
		`"holidays":["2026-05-01T00:00:00Z","2026-10-12T00:00:00Z"],` +
		`"periodRanges":[{"start":"2026-02-02T00:00:00Z","end":"2026-04-17T00:00:00Z"},` +
		`{"start":"2026-04-20T00:00:00Z","end":"2026-07-17T00:00:00Z"},` +
		`{"start":"2026-07-27T00:00:00Z","end":"2026-11-27T00:00:00Z"}]}`
	if rec := doRequest(t, h, http.MethodPut, "/v1/schoolyears/"+id, adminSubject, holidays); rec.Code != http.StatusOK {
		t.Fatalf("holidays edit = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Close is idempotent; closed years accept no edits.
	if rec := doRequest(t, h, http.MethodPost, "/v1/schoolyears/"+id+"/close", adminSubject, ""); rec.Code != http.StatusOK {
		t.Fatalf("close = %d, want 200", rec.Code)
	}
	if rec := doRequest(t, h, http.MethodPost, "/v1/schoolyears/"+id+"/close", adminSubject, ""); rec.Code != http.StatusOK {
		t.Fatalf("re-close = %d, want 200", rec.Code)
	}
	if rec := doRequest(t, h, http.MethodPut, "/v1/schoolyears/"+id, adminSubject, holidays); rec.Code != http.StatusBadRequest {
		t.Fatalf("closed edit = %d, want 400", rec.Code)
	}
	if rec := doRequest(t, h, http.MethodPost, "/v1/schoolyears", adminSubject, yearBody); rec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate = %d, want 400", rec.Code)
	}
}

func TestSchoolYearValidationCodes(t *testing.T) {
	h := yearsStack(t)
	for _, tc := range []struct{ name, body, code string }{
		{"bad periods", `{"year":2026,"periods":9,"startDate":"2026-02-02T00:00:00Z","endDate":"2026-11-27T00:00:00Z","periodRanges":[]}`, "schoolyears.err_invalid_period_ranges"},
		{"bad dates", `{"year":2026,"periods":1,"startDate":"2026-11-27T00:00:00Z","endDate":"2026-02-02T00:00:00Z","periodRanges":[{"start":"2026-02-02T00:00:00Z","end":"2026-11-27T00:00:00Z"}]}`, "schoolyears.err_invalid_dates"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, http.MethodPost, "/v1/schoolyears", adminSubject, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if got := errorCode(t, rec); got != tc.code {
				t.Errorf("code = %q, want %q", got, tc.code)
			}
		})
	}
}

func TestCompleteEmptyRangesOnceStarted(t *testing.T) {
	h, yearsSvc, classesSvc, _ := enrollStack(t)
	ctx := context.Background()

	// Legacy year without ranges, already started.
	y, err := yearsSvc.Create(ctx, schoolyears.SchoolYear{
		Year: 2026, Periods: 2,
		StartDate: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := classesSvc.Create(ctx, classes.ClassGroup{SchoolYearID: y.ID, Grade: 9, GroupNo: 1}); err != nil {
		t.Fatalf("group: %v", err)
	}
	fill := `{"year":2026,"periods":2,"startDate":"2026-02-02T00:00:00Z","endDate":"2026-11-27T00:00:00Z",` +
		`"periodRanges":[{"start":"2026-02-02T00:00:00Z","end":"2026-06-12T00:00:00Z"},` +
		`{"start":"2026-07-06T00:00:00Z","end":"2026-11-27T00:00:00Z"}]}`
	if rec := doRequest(t, h, http.MethodPut, "/v1/schoolyears/"+y.ID, adminSubject, fill); rec.Code != http.StatusOK {
		t.Fatalf("complete = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	// Changing them afterwards is frozen again.
	swap := `{"year":2026,"periods":2,"startDate":"2026-02-02T00:00:00Z","endDate":"2026-11-27T00:00:00Z",` +
		`"periodRanges":[{"start":"2026-02-02T00:00:00Z","end":"2026-05-30T00:00:00Z"},` +
		`{"start":"2026-07-06T00:00:00Z","end":"2026-11-27T00:00:00Z"}]}`
	if rec := doRequest(t, h, http.MethodPut, "/v1/schoolyears/"+y.ID, adminSubject, swap); rec.Code != http.StatusBadRequest {
		t.Fatalf("re-change = %d, want 400", rec.Code)
	}
}
