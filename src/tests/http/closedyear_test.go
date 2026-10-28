package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// yearID resolves the fixture year id through the list endpoint.
func yearID(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := doRequest(t, h, http.MethodGet, "/v1/schoolyears", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("years status = %d, want 200", rec.Code)
	}
	var years []struct {
		ID   string `json:"ID"`
		Year int    `json:"Year"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &years); err != nil {
		t.Fatalf("decode years: %v", err)
	}
	for _, y := range years {
		if y.Year == 2026 {
			return y.ID
		}
	}
	t.Fatal("2026 not found")
	return ""
}

// Closing the year freezes writes: no new roll calls, no mark changes,
// no warnings, no teacher-day records. Reads keep working.
func TestClosedYearBlocksWrites(t *testing.T) {
	h := testStack(t)

	openBody, _ := json.Marshal(map[string]any{"classGroupID": realGroupID, "period": 2})
	rec := doRequest(t, h, http.MethodPost, "/v1/sessions/open", teacherSubject, string(openBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("open status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var opened struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &opened); err != nil {
		t.Fatalf("decode session: %v", err)
	}

	rec = doRequest(t, h, http.MethodPost, "/v1/sessions/"+opened.ID+"/marks", teacherSubject,
		`{"studentID":"`+studentSubject+`","mark":"absence"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("mark status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	warnBody, _ := json.Marshal(map[string]any{
		"studentIDs":  []string{studentSubject},
		"classID":     realGroupID,
		"gravity":     "severe",
		"title":       "T",
		"description": "D",
	})
	rec = doRequest(t, h, http.MethodPost, "/v1/warnings/batch", teacherSubject, string(warnBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("batch status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var issued []struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &issued); err != nil {
		t.Fatalf("decode warnings: %v", err)
	}

	rec = doRequest(t, h, http.MethodPost, "/v1/schoolyears/"+yearID(t, h)+"/close", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("close status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	blocked := []struct {
		method, target, body, subject string
	}{
		{http.MethodPost, "/v1/sessions/open", string(openBody), teacherSubject},
		{http.MethodPost, "/v1/sessions/" + opened.ID + "/marks", `{"studentID":"` + studentSubject + `","mark":"absence"}`, teacherSubject},
		{http.MethodPost, "/v1/warnings/batch", string(warnBody), teacherSubject},
		{http.MethodPut, "/v1/warnings/" + issued[0].ID, `{"title":"Otro"}`, teacherSubject},
		{http.MethodPost, "/v1/teachers/attendance", `{"day":"2026-03-10","marks":[]}`, adminSubject},
	}
	for _, c := range blocked {
		rec := doRequest(t, h, c.method, c.target, c.subject, c.body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s status = %d, want 400", c.method, c.target, rec.Code)
			continue
		}
		if code := errorCode(t, rec); code != "schoolyears.err_year_closed" {
			t.Errorf("%s %s code = %q, want schoolyears.err_year_closed", c.method, c.target, code)
		}
	}

	// Reads still flow on a closed year.
	rec = doRequest(t, h, http.MethodGet, "/v1/sessions/"+opened.ID, teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Errorf("session detail status = %d, want 200", rec.Code)
	}
}

// A bare ?weekday= lists the whole day across groups (day-wide views);
// only a fully unfiltered call is rejected.
func TestScheduleWeekdayOnlyListsDay(t *testing.T) {
	h := scheduleStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/schedule?weekday=1", teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("weekday status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var entries []struct {
		Weekday int `json:"Weekday"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	if len(entries) == 0 {
		t.Error("entries = [], want the Monday fixture slot")
	}
}
