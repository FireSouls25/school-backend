package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"grade/src/core/roles"
	"grade/src/core/schedules"
	"grade/src/core/schoolyears"
	"grade/src/core/tattendance"
	"grade/src/core/teachers"
	httpapi "grade/src/platform/http"
)

// Monday 2026-05-11 (verified weekday): schools teach Mondays, so the
// expected list is non-empty unless the holiday logic clears it.
const holidayMonday = "2026-05-11"

func holidayStack(t *testing.T) (http.Handler, string) {
	t.Helper()
	ctx := context.Background()
	roleSvc := roles.NewService(roles.NewMemoryStore())
	yearsSvc := schoolyears.NewService(schoolyears.NewMemoryStore())
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
	_, err := yearsSvc.Create(ctx, schoolyears.SchoolYear{
		Year: 2026, Periods: 2,
		StartDate:   time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC),
		EndDate:     time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC),
		Holidays:    []time.Time{time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)},
		NoClassDays: []time.Time{time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)},
		PeriodRanges: []schoolyears.PeriodRange{
			{Start: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)},
			{Start: time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)},
		},
	})
	must(err)
	prof, err := teachersSvc.Create(ctx, teachers.Teacher{
		Names: "Carlos", Surnames: "Mendoza", DocumentID: "79800111",
	})
	must(err)
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
		Years:            yearsSvc,
		Teachers:         teachersSvc,
		Schedules:        schedulesSvc,
		TAttendance:      taSvc,
	}, newI18n(t))
	return h, prof.ID
}

func TestHolidayRollSkipsExpectedKeepsRecorded(t *testing.T) {
	h, profID := holidayStack(t)

	// Holiday Monday: nobody expected.
	rec := doRequest(t, h, http.MethodGet, "/v1/teachers/roll?date="+holidayMonday, adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("roll = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || len(rows) != 0 {
		t.Fatalf("rows = %s, want empty", rec.Body.String())
	}

	// Explicit rolls stay allowed on holidays and remain visible.
	rec = doRequest(t, h, http.MethodPost, "/v1/teachers/attendance", adminSubject,
		`{"day":"`+holidayMonday+`","marks":[{"teacherID":"`+profID+`","status":"present"}]}`)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("record = %d, want 200/201: %s", rec.Code, rec.Body.String())
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/teachers/roll?date="+holidayMonday, adminSubject, "")
	rows = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("rows after record = %s, want 1 recorded", rec.Body.String())
	}
	if rows[0]["Recorded"] != true {
		t.Fatalf("row = %v, want recorded", rows[0])
	}
}

func TestOfficialHolidaysSeed(t *testing.T) {
	h, _ := holidayStack(t)
	rec := doRequest(t, h, http.MethodGet, "/v1/holidays?year=2026", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("holidays = %d, want 200", rec.Code)
	}
	var out struct {
		Year     int      `json:"year"`
		Holidays []string `json:"holidays"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, d := range out.Holidays {
		if d == "2026-05-01" {
			found = true
		}
	}
	if !found {
		t.Fatalf("holidays = %v, want 2026-05-01 (Ley 51)", out.Holidays)
	}
	if rec := doRequest(t, h, http.MethodGet, "/v1/holidays?year=1800", adminSubject, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad year = %d, want 400", rec.Code)
	}
}

func TestNoClassDaysRoundTrip(t *testing.T) {
	h := yearsStack(t)
	body := `{"year":2026,"periods":1,"startDate":"2026-02-02T00:00:00Z","endDate":"2026-11-27T00:00:00Z",` +
		`"holidays":["2026-05-01T00:00:00Z"],"noClassDays":["2026-09-07T00:00:00Z"],` +
		`"periodRanges":[{"start":"2026-02-02T00:00:00Z","end":"2026-11-27T00:00:00Z"}]}`
	rec := doRequest(t, h, http.MethodPost, "/v1/schoolyears", adminSubject, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	id, _ := created["ID"].(string)
	rec = doRequest(t, h, http.MethodGet, "/v1/schoolyears/"+id, adminSubject, "")
	var y struct {
		Holidays    []string `json:"Holidays"`
		NoClassDays []string `json:"NoClassDays"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &y); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(y.Holidays) != 1 || len(y.NoClassDays) != 1 {
		t.Fatalf("holidays=%v noClass=%v, want 1 each", y.Holidays, y.NoClassDays)
	}
	// Out-of-range no-class day fails like a bad holiday.
	bad := `{"year":2027,"periods":1,"startDate":"2027-02-01T00:00:00Z","endDate":"2027-11-27T00:00:00Z",` +
		`"noClassDays":["2028-01-05T00:00:00Z"],` +
		`"periodRanges":[{"start":"2027-02-01T00:00:00Z","end":"2027-11-27T00:00:00Z"}]}`
	if rec := doRequest(t, h, http.MethodPost, "/v1/schoolyears", adminSubject, bad); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad no-class = %d, want 400", rec.Code)
	}
}
