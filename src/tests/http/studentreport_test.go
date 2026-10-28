package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The student reads their own report but never another student's.
func TestPermissionReasonTravelsWithTheMark(t *testing.T) {
	h := testStack(t)

	body, _ := json.Marshal(map[string]any{
		"studentID": studentSubject, "mark": "absence-permission", "note": "Cita médica",
	})
	rec := doRequest(t, h, http.MethodPost, "/v1/sessions/"+sessionRouteID+"/marks", teacherSubject, string(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("mark status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/statistics/student/"+studentSubject, adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("report status = %d, want 200", rec.Code)
	}
	var report struct {
		Sessions []struct {
			SessionID string `json:"SessionID"`
			Mark      string `json:"Mark"`
			Note      string `json:"Note"`
		} `json:"Sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	found := false
	for _, s := range report.Sessions {
		if s.Mark != "absence-permission" {
			continue
		}
		found = true
		if s.Note != "Cita médica" {
			t.Errorf("note = %q, want the recorded reason", s.Note)
		}
	}
	if !found {
		t.Error("no absence-permission session found in the report")
	}
}

func TestStudentReadsOwnReportOnly(t *testing.T) {
	h := testStack(t)

	rec := doRequest(t, h, http.MethodGet, "/v1/statistics/student/"+studentSubject, studentSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("own report status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/statistics/student/"+otherStudent, studentSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("other report status = %d, want 403", rec.Code)
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/statistics/student/"+studentSubject, otherStudent, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("peer reading report status = %d, want 403", rec.Code)
	}
}
