package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// markBody posts one roll-call mark and returns the session revision id.
func markBody(t *testing.T, h http.Handler, sessionID, studentID, mark string) {
	t.Helper()
	var payload map[string]string
	if mark == "" {
		payload = map[string]string{"studentID": studentID}
	} else {
		payload = map[string]string{"studentID": studentID, "mark": mark}
	}
	body, _ := json.Marshal(payload)
	rec := doRequest(t, h, http.MethodPost, "/v1/sessions/"+sessionID+"/marks", teacherSubject, string(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("mark %q status = %d, want 201: %s", mark, rec.Code, rec.Body.String())
	}
}

func openDaySession(t *testing.T, h http.Handler) string {
	t.Helper()
	rec := doRequest(t, h, http.MethodPost, "/v1/sessions/open", teacherSubject, `{"classGroupID":"`+realGroupID+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("open status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var sess map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	id, _ := sess["ID"].(string)
	if id == "" {
		t.Fatal("session without ID")
	}
	return id
}

func TestAbsenceEnqueuesGuardianNotice(t *testing.T) {
	h := testStack(t)

	// Student with a dialable guardian phone, enrolled in the group.
	rec := doRequest(t, h, http.MethodPost, "/v1/students", adminSubject,
		`{"names":"Prueba","surnames":"Aviso","classID":"9-1","documentID":"99900011","caregiver":{"names":"Mamá","phone":"3001234567"}}`)
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
	sessID := openDaySession(t, h)
	markBody(t, h, sessID, stID, "absence")

	pending := lastNotifyStore.Pending()
	if len(pending) != before+1 {
		t.Fatalf("pending = %d, want one new notice", len(pending)-before)
	}
	n := pending[len(pending)-1]
	if n.Template != "falta_hoy" || n.To != "+573001234567" {
		t.Errorf("notice = %+v, want falta_hoy to +573001234567", n)
	}

	// Same mark again: the session itself rejects no-change repeats.
	body, _ := json.Marshal(map[string]string{"studentID": stID, "mark": "absence"})
	rec2 := doRequest(t, h, http.MethodPost, "/v1/sessions/"+sessID+"/marks", teacherSubject, string(body))
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("repeat mark status = %d, want 400", rec2.Code)
	}
	if got := len(lastNotifyStore.Pending()); got != len(pending) {
		t.Errorf("pending after double mark = %d, want still %d", got, len(pending))
	}

	// Corrections back to present and justified absences stay silent.
	markBody(t, h, sessID, stID, "")
	markBody(t, h, sessID, stID, "absence-permission")
	if got := len(lastNotifyStore.Pending()); got != len(pending) {
		t.Errorf("pending after correction = %d, want still %d", got, len(pending))
	}

	// Late marks notify with their own template.
	markBody(t, h, sessID, stID, "late")
	pending = lastNotifyStore.Pending()
	last := pending[len(pending)-1]
	if last.Template != "atraso_hoy" {
		t.Errorf("late template = %q, want atraso_hoy", last.Template)
	}
}

func TestLateRecurringNotifiesOnceAMonth(t *testing.T) {
	h := testStack(t)

	rec := doRequest(t, h, http.MethodPost, "/v1/students", adminSubject,
		`{"names":"Tarde","surnames":"Siempre","classID":"9-1","documentID":"99900022","caregiver":{"names":"Mamá","phone":"3007654321"}}`)
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
	// Three late marks across three sessions of the same group.
	for range 3 {
		rec := doRequest(t, h, http.MethodPost, "/v1/sessions/open", teacherSubject, `{"classGroupID":"`+realGroupID+`"}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("open status = %d, want 201: %s", rec.Code, rec.Body.String())
		}
		var sess map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
			t.Fatalf("decode session: %v", err)
		}
		markBody(t, h, sess["ID"].(string), stID, "late")
	}
	rec = doRequest(t, h, http.MethodPost, "/v1/notify/late-recurring", teacherSubject, `{"studentID":"`+stID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("recurring status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode recurring: %v", err)
	}
	if out["lates"] != float64(3) || out["notified"] != true {
		t.Errorf("recurring = %v, want 3 lates notified", out)
	}
	// Second call this month: counted again, not re-sent.
	rec = doRequest(t, h, http.MethodPost, "/v1/notify/late-recurring", teacherSubject, `{"studentID":"`+stID+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("recurring status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	out = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode recurring: %v", err)
	}
	if out["notified"] != false {
		t.Errorf("recurring = %v, want notified false on repeat", out)
	}
}
