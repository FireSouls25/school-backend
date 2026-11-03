package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestWarningModerateEnqueuesGuardianNotice(t *testing.T) {
	h := testStack(t)

	rec := doRequest(t, h, http.MethodPost, "/v1/students", adminSubject,
		`{"names":"Falta","surnames":"Grave","classID":"9-1","documentID":"99900033","caregiver":{"names":"Papá","phone":"3019876543"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create student status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var st map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode student: %v", err)
	}
	stID, _ := st["ID"].(string)
	rec = doRequest(t, h, http.MethodPost, "/v1/enrollments", adminSubject,
		`{"studentID":"`+stID+`","classGroupID":"`+realGroupID+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enroll status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	before := len(lastNotifyStore.Pending())
	rec = doRequest(t, h, http.MethodPost, "/v1/warnings/batch", teacherSubject,
		`{"classID":"`+realGroupID+`","studentIDs":["`+stID+`"],"title":"Pelea","gravity":"moderate","description":"Empujó a otro"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("batch status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	after := lastNotifyStore.Pending()
	if len(after) != before+1 {
		t.Fatalf("pending = %d, want one new notice", len(after)-before)
	}
	if after[len(after)-1].Template != "llamado_atencion" {
		t.Errorf("template = %q, want llamado_atencion", after[len(after)-1].Template)
	}

	// Mild stays in the console: no guardian ping.
	rec = doRequest(t, h, http.MethodPost, "/v1/warnings/batch", teacherSubject,
		`{"classID":"`+realGroupID+`","studentIDs":["`+stID+`"],"title":"Chiste","gravity":"mild","description":"Interrumpió"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("mild batch status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if got := len(lastNotifyStore.Pending()); got != len(after) {
		t.Errorf("pending after mild = %d, want still %d", got, len(after))
	}
}
