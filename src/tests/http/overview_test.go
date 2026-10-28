package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The overview aggregate powers the dashboard charts and the year
// export: distinct roll-call days, real mark tallies, llamados by
// gravity and the per-salon breakdown, scoped to the year or to one
// period.
func TestStatisticsOverview(t *testing.T) {
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
		"gravity":     "moderate",
		"title":       "T",
		"description": "D",
	})
	rec = doRequest(t, h, http.MethodPost, "/v1/warnings/batch", teacherSubject, string(warnBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("batch status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var out struct {
		Year   int      `json:"Year"`
		Period int      `json:"Period"`
		Days   []string `json:"Days"`
		Marks  struct {
			Presences   int `json:"Presences"`
			Absences    int `json:"Absences"`
			StudentDays int `json:"StudentDays"`
		} `json:"Marks"`
		Warnings struct {
			Moderate int `json:"Moderate"`
		} `json:"Warnings"`
		Salons []struct {
			Label string `json:"Label"`
			Days  int    `json:"Days"`
		} `json:"Salons"`
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/statistics/overview?year=2026", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("overview status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if len(out.Days) == 0 {
		t.Error("Days empty, want at least the roll call just taken")
	}
	if out.Marks.Absences < 1 || out.Marks.StudentDays < 1 {
		t.Errorf("Marks = %+v, want the recorded absence folded", out.Marks)
	}
	if out.Warnings.Moderate < 1 {
		t.Errorf("Warnings = %+v, want the issued llamado counted", out.Warnings)
	}
	if len(out.Salons) == 0 || out.Salons[0].Days < 1 {
		t.Errorf("Salons = %+v, want the group breakdown", out.Salons)
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/statistics/overview?year=2026&period=2", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("period overview status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var per struct {
		Period int      `json:"Period"`
		Days   []string `json:"Days"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &per); err != nil {
		t.Fatalf("decode period overview: %v", err)
	}
	if per.Period != 2 || len(per.Days) == 0 {
		t.Errorf("period overview = %+v, want period 2 with days", per)
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/statistics/overview?year=2031", adminSubject, "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown year status = %d, want 404", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/statistics/overview?year=2026", studentSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("teacher status = %d, want 403", rec.Code)
	}
}
