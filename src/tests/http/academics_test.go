package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

type academicAlertJSON struct {
	ID        string `json:"ID"`
	StudentID string `json:"StudentID"`
	Reason    string `json:"Reason"`
	Status    string `json:"Status"`
}

func raiseAlert(t *testing.T, h http.Handler, subject, target, reason string) (int, []academicAlertJSON) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"studentIDs": []string{target}, "reason": reason})
	rec := doRequest(t, h, http.MethodPost, "/v1/academic-alerts", subject, string(body))
	var out []academicAlertJSON
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestTeacherRaisesAcademicAlert(t *testing.T) {
	h := testStack(t)

	code, out := raiseAlert(t, h, teacherSubject, studentSubject, "Acumulación de faltas graves")
	if code != http.StatusCreated {
		t.Fatalf("raise status = %d, want 201", code)
	}
	if len(out) != 1 || out[0].Status != "open" || out[0].Reason == "" {
		t.Errorf("alert = %+v, want one open alert with reason", out)
	}
	if out[0].StudentID != studentSubject {
		t.Errorf("student = %q, want %q", out[0].StudentID, studentSubject)
	}
}

func TestRaiseAcademicAlertValidates(t *testing.T) {
	h := testStack(t)
	fake := "00000000-0000-0000-0000-000000000000"

	if code, _ := raiseAlert(t, h, teacherSubject, studentSubject, ""); code != http.StatusBadRequest {
		t.Errorf("empty reason status = %d, want 400", code)
	}
	body, _ := json.Marshal(map[string]any{"studentIDs": []string{}, "reason": "x"})
	rec := doRequest(t, h, http.MethodPost, "/v1/academic-alerts", teacherSubject, string(body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty batch status = %d, want 400", rec.Code)
	}
	if code, _ := raiseAlert(t, h, teacherSubject, fake, "x"); code != http.StatusNotFound {
		t.Errorf("unknown student status = %d, want 404", code)
	}
}

func TestSecretaryCannotRaiseAcademicAlert(t *testing.T) {
	h, secretary := secretaryStack(t)

	if code, _ := raiseAlert(t, h, secretary, studentSubject, "x"); code != http.StatusForbidden {
		t.Errorf("secretary raise status = %d, want 403", code)
	}
}

func TestAcademicAlertReviewFlow(t *testing.T) {
	h, secretary := secretaryStack(t)

	code, out := raiseAlert(t, h, teacherSubject, studentSubject, "Requiere seguimiento")
	if code != http.StatusCreated {
		t.Fatalf("raise status = %d, want 201", code)
	}
	id := out[0].ID

	// Secretary and admin read the open alert.
	for _, sub := range []string{adminSubject, secretary} {
		rec := doRequest(t, h, http.MethodGet, "/v1/academic-alerts", sub, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("list status = %d, want 200", rec.Code)
		}
		var list []academicAlertJSON
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		if len(list) != 1 || list[0].ID != id {
			t.Errorf("list = %+v, want the open alert", list)
		}
	}

	// Only admins review: teacher and secretary are denied.
	for _, sub := range []string{teacherSubject, secretary} {
		rec := doRequest(t, h, http.MethodPost, "/v1/academic-alerts/"+id+"/review", sub, "")
		if rec.Code != http.StatusForbidden {
			t.Errorf("review by %q status = %d, want 403", sub, rec.Code)
		}
	}

	rec := doRequest(t, h, http.MethodPost, "/v1/academic-alerts/"+id+"/review", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin review status = %d, want 200", rec.Code)
	}
	var reviewed academicAlertJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &reviewed); err != nil {
		t.Fatalf("decode reviewed: %v", err)
	}
	if reviewed.Status != "reviewed" {
		t.Errorf("status = %q, want reviewed", reviewed.Status)
	}

	// Reviewed twice is a conflict, and it leaves the open list.
	rec = doRequest(t, h, http.MethodPost, "/v1/academic-alerts/"+id+"/review", adminSubject, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("second review status = %d, want 409", rec.Code)
	}
	rec = doRequest(t, h, http.MethodGet, "/v1/academic-alerts", adminSubject, "")
	var open []academicAlertJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &open); err != nil {
		t.Fatalf("decode open: %v", err)
	}
	if len(open) != 0 {
		t.Errorf("open = %+v, want empty after review", open)
	}
}

func TestStudentReadsOwnAlerts(t *testing.T) {
	h := testStack(t)

	code, _ := raiseAlert(t, h, teacherSubject, studentSubject, "Seguimiento")
	if code != http.StatusCreated {
		t.Fatalf("raise status = %d, want 201", code)
	}

	rec := doRequest(t, h, http.MethodGet, "/v1/students/"+studentSubject+"/alerts", studentSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("own alerts status = %d, want 200", rec.Code)
	}
	var own []academicAlertJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &own); err != nil {
		t.Fatalf("decode own: %v", err)
	}
	if len(own) != 1 {
		t.Errorf("own = %+v, want the alert", own)
	}

	rec = doRequest(t, h, http.MethodGet, "/v1/students/"+otherStudent+"/alerts", studentSubject, "")
	if rec.Code != http.StatusForbidden {
		t.Errorf("peer alerts status = %d, want 403", rec.Code)
	}
}
