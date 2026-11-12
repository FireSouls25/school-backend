package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Contact, guardian and health data are sensitive (Ley 1581): a teacher
// reads a student's identity only; whoever manages the record reads all.
func TestStudentProfileRedactedForTeachers(t *testing.T) {
	h, studentID := photoStack(t)

	read := func(subject string) map[string]any {
		t.Helper()
		rec := doRequest(t, h, http.MethodGet, "/v1/students/"+studentID, subject, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out
	}

	teacher := read(teacherSubject)
	if teacher["Names"] != "Ana" || teacher["ClassID"] != "9-1" {
		t.Errorf("teacher identity = %v, want names and class", teacher)
	}
	if teacher["DocumentID"] != "" {
		t.Errorf("teacher DocumentID = %v, want redacted", teacher["DocumentID"])
	}
	if cg, _ := teacher["Caregiver"].(map[string]any); cg["Names"] != "" {
		t.Errorf("teacher Caregiver = %v, want redacted", cg)
	}

	admin := read(adminSubject)
	if admin["DocumentID"] != "1234567890" {
		t.Errorf("admin DocumentID = %v, want full record", admin["DocumentID"])
	}
	if cg, _ := admin["Caregiver"].(map[string]any); cg["Names"] != "María" {
		t.Errorf("admin Caregiver = %v, want full record", cg)
	}
}
