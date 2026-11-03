package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestObservationsFlow(t *testing.T) {
	h := scheduleStack(t)

	// Fixed catalog: 65 stable ids with server-decided kinds.
	rec := doRequest(t, h, http.MethodGet, "/v1/observations/catalog", adminSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("catalog status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var cat []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cat); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(cat) != 65 {
		t.Fatalf("catalog len = %d, want 65", len(cat))
	}
	kinds := map[float64]int{}
	for _, s := range cat {
		kinds[s["Kind"].(float64)]++
	}
	if kinds[1] != 14 || kinds[2] != 43 || kinds[3] != 8 {
		t.Fatalf("kinds = %v, want 14/43/8", kinds)
	}
}

func TestRecordObservationFlow(t *testing.T) {
	h := testStack(t)

	// Unknown situation: rejected before touching the store.
	rec := doRequest(t, h, http.MethodPost, "/v1/observations", teacherSubject,
		`{"studentID":"`+studentSubject+`","situationID":"s9-99"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad situation status = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	// Record one: kind comes from the server, snapshots freeze names.
	rec = doRequest(t, h, http.MethodPost, "/v1/observations", teacherSubject,
		`{"studentID":"`+studentSubject+`","classGroupID":"`+realGroupID+`","situationID":"s2-05","note":"empujó en el descanso"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("record status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode observation: %v", err)
	}
	if created["Kind"] != float64(2) {
		t.Errorf("Kind = %v, want 2 (server-decided)", created["Kind"])
	}
	if created["StudentName"] != "Ana Gómez" {
		t.Errorf("StudentName = %v, want frozen roster name", created["StudentName"])
	}
	if created["ClassLabel"] != "9-1" {
		t.Errorf("ClassLabel = %v, want 9-1", created["ClassLabel"])
	}

	// Mine lists it back, newest first.
	rec = doRequest(t, h, http.MethodGet, "/v1/observations", teacherSubject, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("mine status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var mine []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &mine); err != nil {
		t.Fatalf("decode mine: %v", err)
	}
	if len(mine) != 1 || mine[0]["ID"] != created["ID"] {
		t.Errorf("mine = %v, want the recorded row", mine)
	}

	// Future day: rejected.
	rec = doRequest(t, h, http.MethodPost, "/v1/observations", teacherSubject,
		`{"studentID":"`+studentSubject+`","situationID":"s1-01","date":"2999-01-01"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("future status = %d, want 400", rec.Code)
	}
}
